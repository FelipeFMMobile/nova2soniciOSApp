package local.sts.profile;

import com.fasterxml.jackson.databind.*;
import com.fasterxml.jackson.core.JsonParser;
import java.nio.charset.StandardCharsets;
import java.security.*;
import java.time.*;
import java.util.*;
import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;

// v2 has its own canonicalization: Go JSON quoting, sorted payload keys, integer
// numbers only. Business contracts here contain no floating point values.
final class Envelope {
 static final ObjectMapper JSON=new ObjectMapper().enable(JsonParser.Feature.STRICT_DUPLICATE_DETECTION);
 static final List<String> CONTEXT=List.of("version","audience","tool","payload_hash","idempotency_key","confirmed","expires_at","identity","signature");
 static final List<String> IDENTITY=List.of("subject","client_code","session_ref","device_id","scopes");
 record Verified(JsonNode payload,JsonNode identity,String key,boolean confirmed,String hash) {}
 static Verified verify(String audience,String tool,Map<String,Object> args) {
  return verifyAt(audience,tool,args,Instant.now().getEpochSecond());
 }
 static Verified verifyAt(String audience,String tool,Map<String,Object> args,long now) {
  try {
   JsonNode e=JSON.valueToTree(args); exact(e,Set.of("payload","host_context"));
   JsonNode h=e.get("host_context"),p=e.get("payload"),i=h.get("identity"); exact(h,new HashSet<>(CONTEXT)); exact(i,new HashSet<>(IDENTITY));
   if(!p.isObject()||h.path("version").asInt()!=2||!audience.equals(h.path("audience").asText())||!tool.equals(h.path("tool").asText())||!h.path("expires_at").isIntegralNumber()||h.path("expires_at").asLong()<=now||h.path("expires_at").asLong()>now+60||!h.path("confirmed").isBoolean()) deny();
   for(String field:List.of("subject","device_id")) if(!i.path(field).asText().matches("[A-Za-z0-9][A-Za-z0-9._:-]{0,127}")) deny();
   for(String field:List.of("client_code","session_ref")) if(!i.path(field).isTextual()||!i.path(field).asText().matches("[A-Za-z0-9._:-]{0,128}")) deny();
   if(!i.path("scopes").isArray()) deny();
   for(JsonNode scope:i.path("scopes")) if(!scope.isTextual()) deny();
   String key=h.path("idempotency_key").asText();if(key.isBlank()||key.getBytes(StandardCharsets.UTF_8).length>256) deny();
   String payload=canonical(p);if(payload.getBytes(StandardCharsets.UTF_8).length>16384) deny();
   String hash=hex(MessageDigest.getInstance("SHA-256").digest(payload.getBytes(StandardCharsets.UTF_8)));
   if(!hash.equals(h.path("payload_hash").asText())) deny();
   StringBuilder signed=new StringBuilder("{");
   for(String field:CONTEXT){if(signed.length()>1)signed.append(',');signed.append(quote(field)).append(':');
    if(field.equals("signature"))signed.append("\"\"");
    else if(field.equals("identity")){signed.append('{');boolean first=true;for(String f:IDENTITY){if(!first)signed.append(',');first=false;signed.append(quote(f)).append(':').append(canonical(i.get(f)));}signed.append('}');}
    else signed.append(canonical(h.get(field)));
   } signed.append('}');
   byte[] secret=HexFormat.of().parseHex(System.getenv("STS_MCP_CONTEXT_SECRET")); if(secret.length!=32)deny();
   Mac mac=Mac.getInstance("HmacSHA256");mac.init(new SecretKeySpec(secret,"HmacSHA256"));
   byte[] expected=mac.doFinal(("sts-mcp-envelope-v2\0"+signed).getBytes(StandardCharsets.UTF_8));
   byte[] actual=HexFormat.of().parseHex(h.path("signature").asText());
   if(!MessageDigest.isEqual(expected,actual))deny();
   return new Verified(p,i,key,h.get("confirmed").asBoolean(),hash);
  }catch(Exception ex){throw new IllegalArgumentException("invalid_host_context");}
 }
 static void exact(JsonNode node,Set<String> allowed){if(node==null||!node.isObject())deny();Set<String> keys=new HashSet<>();node.fieldNames().forEachRemaining(keys::add);if(!keys.equals(allowed))deny();}
 static void subset(JsonNode node,Set<String> allowed){if(node==null||!node.isObject())deny();node.fieldNames().forEachRemaining(k->{if(!allowed.contains(k))deny();});}
 static void deny(){throw new IllegalArgumentException("operation_rejected");}
 static String hex(byte[] b){return HexFormat.of().formatHex(b);}
 static String quote(String value){try{return JSON.writeValueAsString(value).replace("<","\\u003c").replace(">","\\u003e").replace("&","\\u0026").replace("\u2028","\\u2028").replace("\u2029","\\u2029");}catch(Exception e){throw new IllegalArgumentException();}}
 static String canonical(JsonNode node){
  if(node==null)deny();
  if(node.isObject()){TreeMap<String,JsonNode> sorted=new TreeMap<>();node.fields().forEachRemaining(e->sorted.put(e.getKey(),e.getValue()));List<String> values=new ArrayList<>();sorted.forEach((k,v)->{if(k.codePoints().anyMatch(c->c>127))deny();values.add(quote(k)+":"+canonical(v));});return "{"+String.join(",",values)+"}";}
  if(node.isArray()){List<String> values=new ArrayList<>();node.forEach(v->values.add(canonical(v)));return "["+String.join(",",values)+"]";}
  if(node.isTextual())return quote(node.asText());
  if(node.isNull()||node.isBoolean()||node.isIntegralNumber())return node.toString();
  deny();return "";
 }
}
