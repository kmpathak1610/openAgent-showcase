-- 000005_fix_source_type.sql — allow website/text for knowledge documents
ALTER TABLE documents DROP CONSTRAINT IF EXISTS documents_source_type_check;
ALTER TABLE documents ADD CONSTRAINT documents_source_type_check CHECK (source_type IN ('upload','url','website','text','integration','generated'));
