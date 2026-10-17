package local.sts.profile;
import com.fasterxml.jackson.databind.JsonNode;
import java.net.*;
import java.net.http.*;
import java.time.Duration;
import java.util.*;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Service;
import org.springframework.transaction.support.TransactionTemplate;
@Service
class ProfileService {
 private final JdbcTemplate db;private final TransactionTemplate tx;
 private final HttpClient http=HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(3)).followRedirects(HttpClient.Redirect.NEVER).build();
 ProfileService(JdbcTemplate db,TransactionTemplate tx){this.db=db;this.tx=tx;}
 private String owner(Envelope.Verified e){
  JsonNode i=e.identity();String code=i.path("client_code").asText();if(!code.matches("[a-f0-9]{64}")||i.path("session_ref").asText().isBlank())Envelope.deny();
  boolean allowed=false;for(JsonNode scope:i.path("scopes"))allowed|=scope.asText().equals("profile:read");if(!allowed)Envelope.deny();
  try{String url=System.getenv("STS_IDENTITY_URL"),token=System.getenv("STS_IDENTITY_SERVICE_TOKEN");if(url==null||token==null)Envelope.deny();
   String body=Envelope.JSON.writeValueAsString(Map.of("session_ref",i.path("session_ref").asText(),"subject",i.path("subject").asText(),"client_code",code,"device_id",i.path("device_id").asText()));
   var request=HttpRequest.newBuilder(URI.create(url+"/internal/session")).timeout(Duration.ofSeconds(5)).header("Authorization","Bearer "+token).header("Content-Type","application/json").POST(HttpRequest.BodyPublishers.ofString(body)).build();
   var response=http.send(request,HttpResponse.BodyHandlers.ofString());if(response.statusCode()!=200)Envelope.deny();JsonNode principal=Envelope.JSON.readTree(response.body());
   if(!principal.path("authenticated").asBoolean()||!code.equals(principal.path("clientCode").asText())||!i.path("subject").asText().equals(principal.path("subject").asText()))Envelope.deny();return code;
  }catch(Exception ex){throw new IllegalArgumentException("authentication_required");}
 }
 private Map<String,Object> read(String code,boolean lock){List<Map<String,Object>> rows=db.queryForList("SELECT name,age,city,gender,profession,version FROM profiles WHERE client_code=?"+(lock?" FOR UPDATE":""),code);if(rows.size()!=1)throw new IllegalArgumentException("profile_not_found");Map<String,Object> result=new LinkedHashMap<>();rows.getFirst().forEach((k,v)->result.put(k.toLowerCase(Locale.ROOT),v));return result;}
 Map<String,Object> get(Envelope.Verified e){Envelope.exact(e.payload(),Set.of());return read(owner(e),false);}
 Map<String,Object> update(Envelope.Verified e){
  String code=owner(e);boolean write=false;for(JsonNode s:e.identity().path("scopes"))write|=s.asText().equals("profile:write");if(!write||!e.confirmed())throw new IllegalArgumentException("confirmation_required");
  JsonNode p=e.payload();Envelope.exact(p,Set.of("expected_version","changes"));if(!p.path("expected_version").isIntegralNumber()||p.path("expected_version").asLong()<1)Envelope.deny();validate(p.path("changes"));
  return tx.execute(status->{Map<String,Object> current=read(code,true);
   List<Map<String,Object>> previous=db.queryForList("SELECT payload_hash,result_json FROM profile_operations WHERE client_code=? AND operation_key=?",code,e.key());
   if(!previous.isEmpty()){Map<String,Object> row=new TreeMap<>(String.CASE_INSENSITIVE_ORDER);row.putAll(previous.getFirst());if(!e.hash().equals(row.get("payload_hash")))throw new IllegalArgumentException("idempotency_conflict");try{return Envelope.JSON.readValue(row.get("result_json").toString(),LinkedHashMap.class);}catch(Exception ex){throw new IllegalStateException();}}
   if(((Number)current.get("version")).longValue()!=p.path("expected_version").asLong())throw new IllegalArgumentException("version_conflict");
   p.get("changes").fields().forEachRemaining(f->current.put(f.getKey(),f.getValue().isNull()?null:f.getValue().isIntegralNumber()?f.getValue().intValue():f.getValue().asText().strip()));
   long version=((Number)current.get("version")).longValue()+1;current.put("version",version);
   db.update("UPDATE profiles SET name=?,age=?,city=?,gender=?,profession=?,version=? WHERE client_code=?",current.get("name"),current.get("age"),current.get("city"),current.get("gender"),current.get("profession"),version,code);
   try{db.update("INSERT INTO profile_operations(client_code,operation_key,payload_hash,result_json) VALUES(?,?,?,?)",code,e.key(),e.hash(),Envelope.JSON.writeValueAsString(current));}catch(Exception ex){throw new IllegalStateException("ledger_failed");}return current;
  });
 }
 static void validate(JsonNode changes){Envelope.subset(changes,Set.of("name","age","city","gender","profession"));if(changes.isEmpty())Envelope.deny();changes.fields().forEachRemaining(f->{String k=f.getKey();JsonNode v=f.getValue();if(v.isNull()&&!k.equals("name"))return;if(k.equals("age")){if(!v.isIntegralNumber()||!v.canConvertToInt()||v.intValue()<0||v.intValue()>130)Envelope.deny();return;}int max=k.equals("gender")?80:120;if(!v.isTextual()||v.asText().codePointCount(0,v.asText().length())>max||k.equals("name")&&v.asText().isBlank()||v.asText().codePoints().anyMatch(Character::isISOControl))Envelope.deny();});}
 Map<String,Object> seed(String code,String name){if(!"development".equals(System.getenv("STS_ENV"))||!code.matches("[a-f0-9]{64}")||!Set.of("Alice Exemplo","Bruno Exemplo","Carla Exemplo").contains(name))Envelope.deny();return tx.execute(status->{Long count=db.queryForObject("SELECT COUNT(*) FROM profiles WHERE client_code=?",Long.class,code);if(count==0)db.update("INSERT INTO profiles(client_code,name,age,city,gender,profession) VALUES(?,?,30,'São Paulo',NULL,'Profissional de teste')",code,name);return Map.of("status","seeded");});}
}
