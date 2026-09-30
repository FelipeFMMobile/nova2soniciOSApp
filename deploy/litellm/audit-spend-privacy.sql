\set ON_ERROR_STOP on

DO $$
DECLARE
    leaked bigint;
BEGIN
    SELECT count(*) INTO leaked
    FROM "LiteLLM_SpendLogs"
    WHERE (messages IS NOT NULL AND messages NOT IN ('{}'::jsonb, '[]'::jsonb, 'null'::jsonb))
       OR (response IS NOT NULL AND response NOT IN ('{}'::jsonb, '[]'::jsonb, 'null'::jsonb))
       OR (proxy_server_request IS NOT NULL AND proxy_server_request NOT IN ('{}'::jsonb, '[]'::jsonb, 'null'::jsonb))
       OR (jsonb_typeof(metadata->'mcp_tool_call_metadata') = 'object'
           AND metadata->'mcp_tool_call_metadata' ?| array['arguments', 'result']);
    IF leaked > 0 THEN
        RAISE EXCEPTION 'SpendLogs privacy gate failed: % rows contain conversation payload fields', leaked;
    END IF;
END $$;

SELECT count(*) AS audited_spend_rows FROM "LiteLLM_SpendLogs";
