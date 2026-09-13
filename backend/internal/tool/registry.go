package tool

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"openagent/internal/domain"
	"openagent/internal/repository"
)

// Registry holds tools (global + org)
type Registry struct {
	repo *repository.DB
}

func NewRegistry(repo *repository.DB) *Registry { return &Registry{repo: repo} }

func (r *Registry) Get(orgID *uuid.UUID, name string) (*domain.Tool, error) {
	// try org-specific first, then global
	if orgID != nil {
		tool, err := r.repo.GetToolByName(orgID, name)
		if err == nil { return tool, nil }
	}
	return r.repo.GetToolByName(nil, name)
}

func (r *Registry) List(orgID uuid.UUID) ([]*domain.Tool, error) {
	return r.repo.ListTools(orgID)
}

func (r *Registry) ValidateInput(tool *domain.Tool, input map[string]any) error {
	if tool.InputSchema == nil { return nil }
	// minimal JSON schema validation: check required fields and types
	schemaBytes, _ := json.Marshal(tool.InputSchema)
	var schema map[string]any
	_ = json.Unmarshal(schemaBytes, &schema)
	required, _ := schema["required"].([]any)
	props, _ := schema["properties"].(map[string]any)
	for _, req := range required {
		key, _ := req.(string)
		val, ok := input[key]
		if !ok {
			return fmt.Errorf("missing required field %s", key)
		}
		if props != nil {
			if propDef, ok := props[key].(map[string]any); ok {
				expectedType, _ := propDef["type"].(string)
				if expectedType != "" && !checkType(val, expectedType) {
					return fmt.Errorf("field %s expects %s", key, expectedType)
				}
				// enum check
				if enumVals, ok := propDef["enum"].([]any); ok {
					found := false
					for _, ev := range enumVals {
						if fmt.Sprintf("%v", ev) == fmt.Sprintf("%v", val) { found = true; break }
					}
					if !found { return fmt.Errorf("field %s value %v not in enum %v", key, val, enumVals) }
				}
			}
		}
	}
	// additional: check no unknown dangerous fields? For now allow extra
	return nil
}

func checkType(v any, expected string) bool {
	switch expected {
	case "string": _, ok := v.(string); return ok
	case "integer": _, ok := v.(float64); return ok // json numbers are float64
	case "number": _, ok := v.(float64); return ok
	case "boolean": _, ok := v.(bool); return ok
	case "array": _, ok := v.([]any); return ok
	case "object": _, ok := v.(map[string]any); return ok
	}
	return true
}

func IsHighRisk(tool *domain.Tool) bool {
	return tool.RiskLevel == "high" || tool.RiskLevel == "critical"
}

func RequiresApproval(tool *domain.Tool) bool {
	if tool.ApprovalPolicy != nil {
		if v, ok := tool.ApprovalPolicy["require_approval"].(bool); ok { return v }
	}
	// default safety: external side effects require approval
	return IsHighRisk(tool) || strings.Contains(tool.Name, "publish") || strings.Contains(tool.Name, "send_") || strings.Contains(tool.Name, "delete")
}
