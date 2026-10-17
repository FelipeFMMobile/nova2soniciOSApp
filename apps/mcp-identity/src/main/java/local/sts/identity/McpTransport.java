package local.sts.identity;

import io.modelcontextprotocol.json.jackson.JacksonMcpJsonMapper;
import io.modelcontextprotocol.server.transport.WebMvcStreamableServerTransportProvider;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/** Keep explicit nulls in signed payloads through MCP's Map conversions. */
@Configuration
class McpTransport {
 @Bean WebMvcStreamableServerTransportProvider webMvcStreamableServerTransportProvider() {
  return WebMvcStreamableServerTransportProvider.builder()
   .jsonMapper(new JacksonMcpJsonMapper(Envelope.JSON))
   .mcpEndpoint("/mcp").build();
 }
}
