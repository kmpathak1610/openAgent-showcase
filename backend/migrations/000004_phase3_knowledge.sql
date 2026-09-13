-- 000004_phase3_knowledge.sql — Knowledge hierarchy + RAG

-- Knowledge sources (upload, website, text, integration)
CREATE TABLE IF NOT EXISTS knowledge_sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    source_type TEXT NOT NULL CHECK (source_type IN ('upload','website','text','integration')),
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','syncing','error','archived')),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_knowledge_sources_org ON knowledge_sources(organization_id);
DROP TRIGGER IF EXISTS trg_knowledge_sources_updated_at ON knowledge_sources;
CREATE TRIGGER trg_knowledge_sources_updated_at BEFORE UPDATE ON knowledge_sources FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Knowledge collections (grouping of documents)
CREATE TABLE IF NOT EXISTS knowledge_collections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, slug)
);
CREATE INDEX IF NOT EXISTS idx_collections_org ON knowledge_collections(organization_id);

-- Extend documents with source/collection/version fields if missing
ALTER TABLE documents ADD COLUMN IF NOT EXISTS collection_id UUID REFERENCES knowledge_collections(id) ON DELETE SET NULL;
ALTER TABLE documents ADD COLUMN IF NOT EXISTS source_id UUID REFERENCES knowledge_sources(id) ON DELETE SET NULL;
ALTER TABLE documents ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1;
ALTER TABLE documents ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'organization' CHECK (scope IN ('organization','project','agent'));
CREATE INDEX IF NOT EXISTS idx_documents_collection ON documents(collection_id);
CREATE INDEX IF NOT EXISTS idx_documents_source ON documents(source_id);
CREATE INDEX IF NOT EXISTS idx_documents_scope ON documents(scope);

-- Document versions (immutable snapshots)
CREATE TABLE IF NOT EXISTS document_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    version INT NOT NULL CHECK (version > 0),
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (document_id, version)
);
CREATE INDEX IF NOT EXISTS idx_doc_versions_doc ON document_versions(document_id);

-- Embeddings (explicit table, also keeps copy in document_chunks.embedding for direct vector search)
CREATE TABLE IF NOT EXISTS embeddings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    document_chunk_id UUID NOT NULL REFERENCES document_chunks(id) ON DELETE CASCADE,
    embedding vector(1536) NOT NULL,
    model TEXT NOT NULL DEFAULT 'text-embedding-3-small',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_embeddings_chunk ON embeddings(document_chunk_id);
CREATE INDEX IF NOT EXISTS idx_embeddings_org ON embeddings(organization_id);
CREATE INDEX IF NOT EXISTS idx_embeddings_vector ON embeddings USING hnsw (embedding vector_cosine_ops);

-- Agent knowledge join (agent ↔ document or collection)
CREATE TABLE IF NOT EXISTS agent_knowledge (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    document_id UUID REFERENCES documents(id) ON DELETE CASCADE,
    collection_id UUID REFERENCES knowledge_collections(id) ON DELETE CASCADE,
    granted_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (document_id IS NOT NULL OR collection_id IS NOT NULL),
    UNIQUE (agent_id, document_id, collection_id)
);
CREATE INDEX IF NOT EXISTS idx_agent_knowledge_agent ON agent_knowledge(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_knowledge_doc ON agent_knowledge(document_id);
CREATE INDEX IF NOT EXISTS idx_agent_knowledge_collection ON agent_knowledge(collection_id);

-- Project knowledge join (project ↔ document or collection)
CREATE TABLE IF NOT EXISTS project_knowledge (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    document_id UUID REFERENCES documents(id) ON DELETE CASCADE,
    collection_id UUID REFERENCES knowledge_collections(id) ON DELETE CASCADE,
    granted_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (document_id IS NOT NULL OR collection_id IS NOT NULL),
    UNIQUE (project_id, document_id, collection_id)
);
CREATE INDEX IF NOT EXISTS idx_project_knowledge_project ON project_knowledge(project_id);
CREATE INDEX IF NOT EXISTS idx_project_knowledge_doc ON project_knowledge(document_id);

-- Trigger for collections updated_at
DROP TRIGGER IF EXISTS trg_collections_updated_at ON knowledge_collections;
CREATE TRIGGER trg_collections_updated_at BEFORE UPDATE ON knowledge_collections FOR EACH ROW EXECUTE FUNCTION update_updated_at();
