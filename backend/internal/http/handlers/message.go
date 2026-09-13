package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"openagent/internal/autonomy"
	"openagent/internal/domain"
	"openagent/internal/http/middleware"
	"openagent/internal/repository"
	"openagent/internal/worker"
	"openagent/internal/ws"
)

type MessageHandler struct {
	db       *repository.DB
	hub      *ws.Hub
	autonomy *autonomy.Service
	worker   *worker.Pool
}

func NewMessageHandler(db *repository.DB, hub *ws.Hub) *MessageHandler { return &MessageHandler{db: db, hub: hub} }

func NewMessageHandlerWithAutonomy(db *repository.DB, hub *ws.Hub, autonomy *autonomy.Service, worker *worker.Pool) *MessageHandler {
	return &MessageHandler{db: db, hub: hub, autonomy: autonomy, worker: worker}
}

var mentionRe = regexp.MustCompile(`@([a-zA-Z0-9_-]+)`)

func (h *MessageHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	q := r.URL.Query()
	channelIDStr := q.Get("channelId")
	if channelIDStr == "" { writeError(w, 400, "VALIDATION_ERROR", "channelId required"); return }
	cid, err := uuid.Parse(channelIDStr)
	if err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid channelId"); return }
	ch, err := h.db.GetChannel(claims.OrganizationID, cid)
	if err == sql.ErrNoRows { writeError(w, 404, "NOT_FOUND", "channel not found"); return }
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	// Enforce channel membership OR org membership for Phase 1? We enforce org member + if private channel enforce member
	if ch.ChannelType == "private" && !h.db.IsChannelMember(cid, claims.UserID) {
		writeError(w, 403, "FORBIDDEN", "not a channel member")
		return
	}
	if _, ok := h.db.IsOrgMember(claims.OrganizationID, claims.UserID); !ok { writeError(w, 403, "FORBIDDEN", "not org member"); return }

	var threadID *uuid.UUID
	if t := q.Get("threadId"); t != "" {
		tid, err := uuid.Parse(t)
		if err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid threadId"); return }
		threadID = &tid
	}
	var before time.Time
	if b := q.Get("before"); b != "" {
		if parsed, err := time.Parse(time.RFC3339, b); err == nil { before = parsed }
	}
	page, pageSize := pagination(r)
	limit := pageSize
	// simple offset via before cursor; page param shifts before via? we use cursor only; but support page via offset? For now use limit
	_ = page
	msgs, err := h.db.ListMessages(claims.OrganizationID, cid, threadID, before, limit, false)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	_ = h.db.HydrateReactions(msgs)
	// enrich sender names (batch fetch could be optimized)
	writeData(w, 200, msgs, &meta{Page: page, PageSize: pageSize, Total: len(msgs)})
}

func (h *MessageHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	var req struct {
		ChannelID string  `json:"channelId"`
		Body      string  `json:"body"`
		ThreadID  *string `json:"threadId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	if req.Body == "" { writeError(w, 400, "VALIDATION_ERROR", "body required"); return }
	cid, err := uuid.Parse(req.ChannelID)
	if err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid channelId"); return }
	ch, err := h.db.GetChannel(claims.OrganizationID, cid)
	if err == sql.ErrNoRows { writeError(w, 404, "NOT_FOUND", "channel not found"); return }
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if ch.ChannelType == "private" && !h.db.IsChannelMember(cid, claims.UserID) {
		writeError(w, 403, "FORBIDDEN", "not a channel member")
		return
	}
	var threadID *uuid.UUID
	if req.ThreadID != nil && *req.ThreadID != "" {
		tid, err := uuid.Parse(*req.ThreadID)
		if err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid threadId"); return }
		// verify thread root exists
		if _, err := h.db.GetMessage(claims.OrganizationID, tid); err != nil { writeError(w, 404, "NOT_FOUND", "thread not found"); return }
		threadID = &tid
	}
	msg, err := h.db.CreateMessage(claims.OrganizationID, cid, threadID, "user", &claims.UserID, req.Body)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	// broadcast
	if h.hub != nil {
		h.hub.BroadcastToOrg(claims.OrganizationID, "message.created", map[string]any{
			"message": msg,
			"channelId": cid.String(),
		})
		h.hub.Broadcast(ws.Event{Type: "message.created", Payload: map[string]any{"message": msg, "channelId": cid.String()}, Room: "channel:" + cid.String()})
	}
	if ch.Name == "general" && ch.ProjectID == nil && msg.SenderType == "user" {
		orgID := claims.OrganizationID
		channelID := ch.ID
		userMsgID := msg.ID
		body := req.Body
		creator := claims.UserID
		if msg.SenderUserID != nil {
			creator = *msg.SenderUserID
		}
		go h.routeGeneral(orgID, channelID, userMsgID, body, creator, threadID)
	}
	// Autonomy wiring: message.created → autonomous agents
	if ch.ProjectID != nil && h.autonomy != nil && msg.SenderType == "user" {
		go h.autonomy.HandleProjectEvent(r.Context(), claims.OrganizationID, *ch.ProjectID, "message.created", map[string]any{"messageId": msg.ID.String(), "channelId": cid.String(), "body": req.Body})
	}
	// Mention resolver: @AgentSlug → create task for assigned agents
	if ch.ProjectID != nil && msg.SenderType == "user" {
		mentions := mentionRe.FindAllStringSubmatch(req.Body, -1)
		seen := map[string]bool{}
		for _, m := range mentions {
			slug := strings.ToLower(strings.TrimSpace(m[1]))
			if slug == "" || seen[slug] {
				continue
			}
			seen[slug] = true
			agents, _ := h.db.ListAgents(claims.OrganizationID)
			for _, ag := range agents {
				if !(strings.EqualFold(ag.Slug, slug) || strings.EqualFold(ag.Name, slug) || strings.EqualFold(strings.ReplaceAll(ag.Name, " ", "-"), slug) || strings.EqualFold(strings.ReplaceAll(ag.Name, " ", "_"), slug)) {
					continue
				}
				// Enforce project assignment
				var assigned bool
				_ = h.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM project_agents WHERE project_id=$1 AND agent_id=$2)`, *ch.ProjectID, ag.ID).Scan(&assigned)
				if !assigned {
					_ = h.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM agent_team_members atm JOIN project_teams pt ON pt.team_id=atm.team_id WHERE pt.project_id=$1 AND atm.agent_id=$2)`, *ch.ProjectID, ag.ID).Scan(&assigned)
				}
				if !assigned {
					continue
				}
				title := truncateTitle(req.Body)
				corrID := msg.ID // use message ID as correlation for dedup
				task, err := h.db.CreateTask(claims.OrganizationID, *ch.ProjectID, &cid, nil, nil, &ag.ID, nil, title, req.Body, "medium", nil, &corrID, claims.UserID)
				if err == nil {
					if h.worker != nil {
						h.worker.Enqueue(worker.Job{
							ID:   task.ID.String(),
							Type: "agent_run",
							Payload: map[string]any{
								"organization_id": claims.OrganizationID.String(),
								"agent_id":        ag.ID.String(),
								"task_id":         task.ID.String(),
								"correlation_id":  corrID.String(),
								"trigger":         "mention",
							},
						})
					}
					if h.hub != nil {
						h.hub.BroadcastToOrg(claims.OrganizationID, "agent.task_created", map[string]any{"task": task, "trigger": "mention", "mention": slug})
					}
				}
				break // one task per mention slug
			}
		}
	}
	writeData(w, 201, msg, nil)
}

func truncateTitle(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > 80 {
		return string(r[:80])
	}
	return s
}

func pickGeneralAgent(agents []*domain.Agent, body string) *domain.Agent {
	if len(agents) == 0 {
		return nil
	}
	mentions := mentionRe.FindAllStringSubmatch(body, -1)
	seen := map[string]bool{}
	for _, m := range mentions {
		slug := strings.ToLower(strings.TrimSpace(m[1]))
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		for _, ag := range agents {
			if ag == nil {
				continue
			}
			// 4-way match parity with project mention path: Slug, Name, Name with -, Name with _
			if strings.EqualFold(ag.Slug, slug) || strings.EqualFold(ag.Name, slug) || strings.EqualFold(strings.ReplaceAll(ag.Name, " ", "-"), slug) || strings.EqualFold(strings.ReplaceAll(ag.Name, " ", "_"), slug) {
				return ag
			}
		}
	}
	for _, ag := range agents {
		if ag != nil && ag.Slug == "openagent-assistant" {
			return ag
		}
	}
	return agents[0]
}

func shouldSkipGeneralRoute(threadID *uuid.UUID) bool {
	return threadID != nil
}

func (h *MessageHandler) routeGeneral(orgID, channelID, userMsgID uuid.UUID, body string, creator uuid.UUID, threadID *uuid.UUID) {
	// Task 4 preserves thread carry: skip creating new top-level Home task for thread replies to avoid task explosion.
	if shouldSkipGeneralRoute(threadID) {
		return
	}
	agents, _ := h.db.ListAgents(orgID)
	if len(agents) == 0 {
		log.Printf("routeGeneral: no agents for org %s, skipping general route", orgID)
		return
	}
	picked := pickGeneralAgent(agents, body)
	if picked == nil {
		log.Printf("routeGeneral: no agent picked for org %s, skipping", orgID)
		return
	}
	homeProject, err := h.db.EnsureHomeProject(orgID, creator)
	if err != nil || homeProject == uuid.Nil {
		log.Printf("routeGeneral: ensure home project failed org %s: %v", orgID, err)
		return
	}
	title := truncateTitle(body)
	corrID := userMsgID
	task, err := h.db.CreateTask(orgID, homeProject, &channelID, nil, nil, &picked.ID, nil, title, body, "medium", nil, &corrID, creator)
	if err != nil {
		log.Printf("routeGeneral: CreateTask failed org %s home %s: %v", orgID, homeProject, err)
		return
	}
	if h.worker != nil {
		h.worker.Enqueue(worker.Job{ID: task.ID.String(), Type: "agent_run", Payload: map[string]any{"organization_id": orgID.String(), "agent_id": picked.ID.String(), "task_id": task.ID.String(), "correlation_id": corrID.String(), "trigger": "general"}})
	}
	if h.hub != nil {
		h.hub.BroadcastToOrg(orgID, "agent.task_created", map[string]any{"task": task, "trigger": "general"})
	}
}

func (h *MessageHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	mid, _ := uuid.Parse(chi.URLParam(r, "id"))
	msg, err := h.db.GetMessage(claims.OrganizationID, mid)
	if err == sql.ErrNoRows { writeError(w, 404, "NOT_FOUND", "message not found"); return }
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	reactions, _ := h.db.ListReactions(mid)
	if len(reactions) > 0 {
		msg.Reactions = make([]domain.Reaction, len(reactions))
		for i, r := range reactions { msg.Reactions[i] = *r }
	}
	msg.ThreadCount = h.db.CountThreadReplies(mid)
	writeData(w, 200, msg, nil)
}

func (h *MessageHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	mid, _ := uuid.Parse(chi.URLParam(r, "id"))
	msg, err := h.db.GetMessage(claims.OrganizationID, mid)
	if err == sql.ErrNoRows { writeError(w, 404, "NOT_FOUND", "message not found"); return }
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if msg.SenderUserID == nil || *msg.SenderUserID != claims.UserID {
		writeError(w, 403, "FORBIDDEN", "only author can edit")
		return
	}
	var req struct{ Body string `json:"body"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	if req.Body == "" { writeError(w, 400, "VALIDATION_ERROR", "body required"); return }
	updated, err := h.db.UpdateMessage(claims.OrganizationID, mid, req.Body)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if h.hub != nil {
		h.hub.BroadcastToOrg(claims.OrganizationID, "message.updated", map[string]any{"message": updated, "channelId": updated.ChannelID.String()})
		h.hub.Broadcast(ws.Event{Type: "message.updated", Payload: map[string]any{"message": updated}, Room: "channel:" + updated.ChannelID.String()})
	}
	writeData(w, 200, updated, nil)
}

func (h *MessageHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	mid, _ := uuid.Parse(chi.URLParam(r, "id"))
	msg, err := h.db.GetMessage(claims.OrganizationID, mid)
	if err == sql.ErrNoRows { writeError(w, 404, "NOT_FOUND", "message not found"); return }
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	// author or org admin can delete
	if msg.SenderUserID == nil || *msg.SenderUserID != claims.UserID {
		role, _ := h.db.IsOrgMember(claims.OrganizationID, claims.UserID)
		if role != "owner" && role != "admin" { writeError(w, 403, "FORBIDDEN", "only author or admin can delete"); return }
	}
	if err := h.db.DeleteMessage(claims.OrganizationID, mid); err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if h.hub != nil {
		h.hub.BroadcastToOrg(claims.OrganizationID, "message.deleted", map[string]any{"messageId": mid.String(), "channelId": msg.ChannelID.String()})
		h.hub.Broadcast(ws.Event{Type: "message.deleted", Payload: map[string]any{"messageId": mid.String()}, Room: "channel:" + msg.ChannelID.String()})
	}
	writeData(w, 200, map[string]any{"deleted": true}, nil)
}

func (h *MessageHandler) ListReplies(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	mid, _ := uuid.Parse(chi.URLParam(r, "id"))
	msg, err := h.db.GetMessage(claims.OrganizationID, mid)
	if err == sql.ErrNoRows { writeError(w, 404, "NOT_FOUND", "message not found"); return }
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	replies, err := h.db.ListMessages(claims.OrganizationID, msg.ChannelID, &mid, time.Time{}, 50, false)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	_ = h.db.HydrateReactions(replies)
	writeData(w, 200, replies, nil)
}

func (h *MessageHandler) AddReaction(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	mid, _ := uuid.Parse(chi.URLParam(r, "id"))
	var req struct{ Emoji string `json:"emoji"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, 400, "VALIDATION_ERROR", "invalid JSON"); return }
	if req.Emoji == "" { writeError(w, 400, "VALIDATION_ERROR", "emoji required"); return }
	// verify message belongs to org
	if _, err := h.db.GetMessage(claims.OrganizationID, mid); err != nil { writeError(w, 404, "NOT_FOUND", "message not found"); return }
	reaction, err := h.db.AddReaction(claims.OrganizationID, mid, claims.UserID, req.Emoji)
	if err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if h.hub != nil {
		h.hub.BroadcastToOrg(claims.OrganizationID, "reaction.added", map[string]any{"messageId": mid.String(), "reaction": reaction})
	}
	writeData(w, 201, reaction, nil)
}

func (h *MessageHandler) RemoveReaction(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.GetClaims(r.Context())
	mid, _ := uuid.Parse(chi.URLParam(r, "id"))
	emoji := chi.URLParam(r, "emoji")
	if emoji == "" { writeError(w, 400, "VALIDATION_ERROR", "emoji required"); return }
	if err := h.db.RemoveReaction(claims.OrganizationID, mid, claims.UserID, emoji); err != nil { writeError(w, 500, "INTERNAL", err.Error()); return }
	if h.hub != nil {
		h.hub.BroadcastToOrg(claims.OrganizationID, "reaction.removed", map[string]any{"messageId": mid.String(), "emoji": emoji, "userId": claims.UserID.String()})
	}
	writeData(w, 200, map[string]any{"removed": true}, nil)
}
