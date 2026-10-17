package local.sts.identity;
import java.time.Instant;
import java.util.*;
import com.fasterxml.jackson.databind.JsonNode;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Service;
import org.springframework.transaction.support.TransactionTemplate;

@Service
class IdentityService {
 private final JdbcTemplate db; private final TransactionTemplate tx; private final IdentityProvider provider;
 IdentityService(JdbcTemplate db,TransactionTemplate tx,IdentityProvider provider){this.db=db;this.tx=tx;this.provider=provider;Crypto.key("STS_CLIENT_CODE_KEY");Crypto.key("STS_TOKEN_ENCRYPTION_KEY");}
 private long now(){return Instant.now().getEpochSecond();}
 private Map<String,Object> one(String sql,Object...args){List<Map<String,Object>> rows=db.queryForList(sql,args);if(rows.size()!=1)Envelope.deny();Map<String,Object> result=new TreeMap<>(String.CASE_INSENSITIVE_ORDER);result.putAll(rows.getFirst());return result;}
 String begin(String device){
  if(!device.matches("[a-f0-9]{64}"))Envelope.deny();
  Long n=db.queryForObject("SELECT COUNT(*) FROM login_transactions WHERE device_id=? AND consumed=FALSE AND expires_at>?",Long.class,device,now());if(n!=null&&n>=5)Envelope.deny();
  String id=Crypto.random(),state=Crypto.random(),nonce=Crypto.random(),verifier=Crypto.random();
  db.update("INSERT INTO login_transactions(id,device_id,state_hash,nonce,verifier_cipher,expires_at) VALUES(?,?,?,?,?,?)",id,device,Crypto.hash(state),nonce,Crypto.encrypt(verifier+":"+state),now()+600);return id;
 }
 Map<String,Object> prepare(String id,String device){Map<String,Object> t=transaction(id,device);String[] secrets=Crypto.decrypt(t.get("verifier_cipher").toString()).split(":");return Map.of("authorization_url",provider.authorize(secrets[1],t.get("nonce").toString(),Crypto.challenge(secrets[0])),"expires_at",t.get("expires_at"),"fake",provider.issuer().equals("urn:sts:fake"));}
 private Map<String,Object> transaction(String id,String device){Map<String,Object> t=one("SELECT * FROM login_transactions WHERE id=? AND device_id=?",id,device);if((boolean)t.get("consumed")||((Number)t.get("expires_at")).longValue()<=now())Envelope.deny();return t;}
 Map<String,Object> complete(String id,String device,String state,String code){
  Map<String,Object> t=tx.execute(status->{Map<String,Object> row=transaction(id,device);if(!Crypto.hash(state).equals(row.get("state_hash")))Envelope.deny();int changed=db.update("UPDATE login_transactions SET consumed=TRUE WHERE id=? AND consumed=FALSE",id);if(changed!=1)Envelope.deny();return row;});
  ProviderTokens tokens=provider.exchange(code,Crypto.decrypt(t.get("verifier_cipher").toString()).split(":")[0],t.get("nonce").toString());
  return tx.execute(status->{Map<String,Object> identity=one("SELECT * FROM identities WHERE issuer=? AND subject=? AND enabled=TRUE",provider.issuer(),tokens.subject());if(!tokens.email().equalsIgnoreCase(identity.get("email").toString()))Envelope.deny();
   String sid=Crypto.random(),access=Crypto.random(),refresh=Crypto.random();long expiry=Math.min(now()+900,tokens.expiresAt());if(expiry<=now())Envelope.deny();
   db.update("INSERT INTO account_sessions(id,identity_id,device_id,access_hash,refresh_hash,cognito_cipher,access_expires,expires_at) VALUES(?,?,?,?,?,?,?,?)",sid,identity.get("id"),device,Crypto.hash(access),Crypto.hash(refresh),Crypto.encrypt(tokens.refresh()),expiry,now()+30L*86400);
   return issued(access,refresh,expiry);
  });
 }
 private Map<String,Object> issued(String access,String refresh,long expiry){return Map.of("access_token",access,"refresh_token",refresh,"expires_at",expiry);}
 Map<String,Object> refresh(String token,String device){
  // One-use refresh rotation is serialized per session; a replay revokes the family.
  return tx.execute(status->{List<Map<String,Object>> replay=db.queryForList("SELECT session_id FROM consumed_refresh_tokens WHERE token_hash=?",Crypto.hash(token));if(!replay.isEmpty()){Object sid=replay.getFirst().values().iterator().next();db.update("UPDATE account_sessions SET revoked=TRUE WHERE id=?",sid);return Map.of("error","session_revoked");}
   Map<String,Object> s=one("SELECT * FROM account_sessions WHERE refresh_hash=? AND device_id=? FOR UPDATE",Crypto.hash(token),device);active(s,false);
   ProviderTokens p=provider.refresh(Crypto.decrypt(s.get("cognito_cipher").toString()));Map<String,Object> identity=one("SELECT * FROM identities WHERE id=? AND enabled=TRUE",s.get("identity_id"));if(!p.subject().equals(identity.get("subject"))||!p.email().equalsIgnoreCase(identity.get("email").toString()))Envelope.deny();
   String access=Crypto.random(),refresh=Crypto.random();long expiry=Math.min(Math.min(now()+900,p.expiresAt()),((Number)s.get("expires_at")).longValue());
   db.update("INSERT INTO consumed_refresh_tokens(token_hash,session_id) VALUES(?,?)",Crypto.hash(token),s.get("id"));
   db.update("UPDATE account_sessions SET access_hash=?,refresh_hash=?,cognito_cipher=?,access_expires=? WHERE id=?",Crypto.hash(access),Crypto.hash(refresh),Crypto.encrypt(p.refresh()),expiry,s.get("id"));return issued(access,refresh,expiry);
  });
 }
 private void active(Map<String,Object>s,boolean access){if((boolean)s.get("revoked")||((Number)s.get("expires_at")).longValue()<=now()||access&&((Number)s.get("access_expires")).longValue()<=now())Envelope.deny();}
 Map<String,Object> introspect(String access,String device){Map<String,Object>s=one("SELECT * FROM account_sessions WHERE access_hash=? AND device_id=?",Crypto.hash(access),device);active(s,true);return principal(s);}
 Map<String,Object> session(String ref,String subject,String code,String device){Map<String,Object>s=one("SELECT * FROM account_sessions WHERE id=? AND device_id=?",ref,device);active(s,true);Map<String,Object> p=principal(s);if(!subject.equals(p.get("subject"))||!code.equals(p.get("clientCode")))Envelope.deny();return p;}
 private Map<String,Object> principal(Map<String,Object>s){Map<String,Object> i=one("SELECT * FROM identities WHERE id=? AND enabled=TRUE",s.get("identity_id"));return Map.of("subject",i.get("id"),"clientCode",i.get("client_code"),"sessionRef",s.get("id"),"deviceId",s.get("device_id"),"authenticated",true);}
 void logout(String ref,String subject,String code,String device){session(ref,subject,code,device);Map<String,Object>s=one("SELECT * FROM account_sessions WHERE id=?",ref);db.update("UPDATE account_sessions SET revoked=TRUE WHERE id=?",ref);provider.revoke(Crypto.decrypt(s.get("cognito_cipher").toString()));}
 void logoutRefresh(String token,String device){
  Map<String,Object> s=one("SELECT * FROM account_sessions WHERE refresh_hash=? AND device_id=?",Crypto.hash(token),device);
  db.update("UPDATE account_sessions SET revoked=TRUE WHERE id=?",s.get("id"));
  provider.revoke(Crypto.decrypt(s.get("cognito_cipher").toString()));
 }
 Map<String,Object> status(JsonNode i){if(i.path("session_ref").asText().isEmpty())return Map.of("status","anonymous");session(i.path("session_ref").asText(),i.path("subject").asText(),i.path("client_code").asText(),i.path("device_id").asText());return Map.of("status","authenticated");}
 Map<String,Object> seed(String fixture,String subject,String email){
  if(!"development".equals(System.getenv("STS_ENV"))||!Set.of("alice","bruno","carla").contains(fixture)||subject.isBlank()||email.isBlank())Envelope.deny();
  return tx.execute(status->{List<Map<String,Object>> rows=db.queryForList("SELECT i.* FROM identities i JOIN fixture_bindings f ON f.identity_id=i.id WHERE f.fixture_key=?",fixture);
   if(!rows.isEmpty()){Map<String,Object> i=new TreeMap<>(String.CASE_INSENSITIVE_ORDER);i.putAll(rows.getFirst());if(!subject.equals(i.get("subject"))||!email.equals(i.get("email")))Envelope.deny();return Map.of("client_code",i.get("client_code"));}
   String id=UUID.randomUUID().toString(),code=Crypto.code(id);db.update("INSERT INTO identities(id,issuer,subject,email,client_code,fixture) VALUES(?,?,?,?,?,TRUE)",id,provider.issuer(),subject,email,code);db.update("INSERT INTO fixture_bindings(fixture_key,identity_id) VALUES(?,?)",fixture,id);return Map.of("client_code",code);
  });
 }
}
