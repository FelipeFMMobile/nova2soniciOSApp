package local.sts.profile;
import java.util.*;
import com.fasterxml.jackson.databind.JsonNode;
import org.springframework.context.annotation.*;
import io.modelcontextprotocol.server.McpServerFeatures;
import io.modelcontextprotocol.spec.McpSchema;
@Configuration
class McpTools {
 @Bean List<McpServerFeatures.SyncToolSpecification> tools(ProfileService service) throws Exception {
  JsonNode specs=Envelope.JSON.readTree(getClass().getResourceAsStream("/contracts/v2/tools.json"));
  List<McpServerFeatures.SyncToolSpecification> result=new ArrayList<>();
  for(JsonNode spec:specs){String name=spec.path("name").asText();
   Map<String,Object> payload=Envelope.JSON.convertValue(spec.get("inputSchema"),Map.class);
   var schema=new McpSchema.JsonSchema("object",Map.of("payload",payload,"host_context",Map.of("type","object")),List.of("payload","host_context"),false,null,null);
   var tool=McpSchema.Tool.builder().name(name).description(spec.path("description").asText()).inputSchema(schema).build();
   result.add(new McpServerFeatures.SyncToolSpecification(tool,(exchange,args)->{
    try{var e=Envelope.verify("profile",name,args);Object value;
     value=switch(name){case "profile.get_me" -> service.get(e);case "profile.update_me" -> service.update(e);default -> throw new IllegalArgumentException();};
     return McpSchema.CallToolResult.builder().addTextContent(Envelope.JSON.writeValueAsString(value)).isError(false).build();
    }catch(Exception ex){String error=ex.getMessage();if(!Set.of("authentication_required","confirmation_required","version_conflict","idempotency_conflict","profile_not_found","invalid_host_context").contains(error==null?"":error))error="operation_rejected";
     return McpSchema.CallToolResult.builder().addTextContent("{\"status\":\"error\",\"error\":"+Envelope.quote(error)+"}").isError(true).build();}
   }));
  }return result;
 }
}
