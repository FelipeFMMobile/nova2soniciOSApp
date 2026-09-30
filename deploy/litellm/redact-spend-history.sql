\set ON_ERROR_STOP on

BEGIN;
UPDATE "LiteLLM_SpendLogs"
SET metadata = jsonb_set(
        metadata,
        '{mcp_tool_call_metadata}',
        (metadata->'mcp_tool_call_metadata') - 'arguments' - 'result',
        false
    ),
    updated_at = CURRENT_TIMESTAMP
WHERE jsonb_typeof(metadata->'mcp_tool_call_metadata') = 'object'
  AND metadata->'mcp_tool_call_metadata' ?| array['arguments', 'result'];
COMMIT;
