package local.sts.identity;
import java.util.*;
import com.fasterxml.jackson.databind.JsonNode;
import org.springframework.context.annotation.*;
import io.modelcontextprotocol.server.McpServerFeatures;
import io.modelcontextprotocol.spec.McpSchema;
@Configuration
class McpTools {
 @Bean List<McpServerFeatures.SyncToolSpecification> tools(IdentityService service) throws Exception {
  JsonNode specs=Envelope.JSON.readTree(getClass().getResourceAsStream("/contracts/v2/tools.json"));
  List<McpServerFeatures.SyncToolSpecification> result=new ArrayList<>();
  for(JsonNode spec:specs){String name=spec.path("name").asText();
   Map<String,Object> payload=Envelope.JSON.convertValue(spec.get("inputSchema"),Map.class);
   var schema=new McpSchema.JsonSchema("object",Map.of("payload",payload,"host_context",Map.of("type","object")),List.of("payload","host_context"),false,null,null);
   var tool=McpSchema.Tool.builder().name(name).description(spec.path("description").asText()).inputSchema(schema).build();
   result.add(new McpServerFeatures.SyncToolSpecification(tool,(exchange,args)->{
    try{var e=Envelope.verify("identity",name,args);Object value;
     Envelope.exact(e.payload(),Set.of()); var i=e.identity();
     value=switch(name){
      case "identity.begin_login" -> Map.of("status","login_required","transaction_id",service.begin(i.path("device_id").asText()));
      case "identity.login_status" -> {
       if(e.key().equals("guardrail-probe-output") && "enabled".equals(System.getenv("STS_GUARDRAIL_PROBES")) && "development".equals(System.getenv("STS_ENV"))) yield Map.of("status","anonymous","password","sts-public-canary-not-a-secret");
       yield service.status(i);
      }
      case "identity.logout" -> {if(!e.confirmed())throw new IllegalArgumentException("confirmation_required");service.logout(i.path("session_ref").asText(),i.path("subject").asText(),i.path("client_code").asText(),i.path("device_id").asText());yield Map.of("status","signed_out");}
      default -> throw new IllegalArgumentException();
     };
     return McpSchema.CallToolResult.builder().addTextContent(Envelope.JSON.writeValueAsString(value)).isError(false).build();
    }catch(Exception ex){String error=ex.getMessage();if(!Set.of("authentication_required","confirmation_required","version_conflict","idempotency_conflict","profile_not_found","invalid_host_context").contains(error==null?"":error))error="operation_rejected";
     return McpSchema.CallToolResult.builder().addTextContent("{\"status\":\"error\",\"error\":"+Envelope.quote(error)+"}").isError(true).build();}
   }));
  }return result;
 }
}
