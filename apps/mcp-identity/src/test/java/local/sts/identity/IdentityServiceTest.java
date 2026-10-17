package local.sts.identity;
import org.junit.jupiter.api.*;
import static org.junit.jupiter.api.Assertions.*;
import org.springframework.jdbc.datasource.embedded.*;
import org.springframework.jdbc.datasource.DataSourceTransactionManager;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.transaction.support.TransactionTemplate;
import java.net.URI;
import java.util.*;
class IdentityServiceTest {
 EmbeddedDatabase db; JdbcTemplate jdbc; IdentityService service;
 final String device="d".repeat(64);
 @BeforeEach void setup(){db=new EmbeddedDatabaseBuilder().generateUniqueName(true).setType(EmbeddedDatabaseType.H2).addScript("db/migration/V1__identity.sql").build();jdbc=new JdbcTemplate(db);service=new IdentityService(jdbc,new TransactionTemplate(new DataSourceTransactionManager(db)),new CognitoProvider());service.seed("alice","alice","alice@example.invalid");}
 @AfterEach void close(){db.shutdown();}
 String state(String id){String url=service.prepare(id,device).get("authorization_url").toString();return URI.create(url).getQuery().split("state=")[1];}
 Map<String,Object> login(){String id=service.begin(device);return service.complete(id,device,state(id),"fake-alice");}
 @Test void loginRotationReplayAndRevocation(){var issued=login();var p=service.introspect(issued.get("access_token").toString(),device);assertEquals(true,p.get("authenticated"));assertEquals(64,p.get("clientCode").toString().length());assertThrows(IllegalArgumentException.class,()->service.introspect(issued.get("access_token").toString(),"e".repeat(64)));var rotated=service.refresh(issued.get("refresh_token").toString(),device);assertThrows(IllegalArgumentException.class,()->service.introspect(issued.get("access_token").toString(),device));assertEquals("session_revoked",service.refresh(issued.get("refresh_token").toString(),device).get("error"));assertThrows(IllegalArgumentException.class,()->service.introspect(rotated.get("access_token").toString(),device));}
 @Test void callbackSingleUseBoundToStateAndDevice(){String id=service.begin(device),state=state(id);assertThrows(IllegalArgumentException.class,()->service.complete(id,device,"wrong","fake-alice"));assertThrows(IllegalArgumentException.class,()->service.complete(id,"e".repeat(64),state,"fake-alice"));service.complete(id,device,state,"fake-alice");assertThrows(IllegalArgumentException.class,()->service.complete(id,device,state,"fake-alice"));}
 @Test void expiredAndInvalidCodes(){String id=service.begin(device),state=state(id);jdbc.update("UPDATE login_transactions SET expires_at=0 WHERE id=?",id);assertThrows(IllegalArgumentException.class,()->service.complete(id,device,state,"fake-alice"));String invalid=service.begin(device),s=state(invalid);assertThrows(IllegalArgumentException.class,()->service.complete(invalid,device,s,"invalid"));assertThrows(IllegalArgumentException.class,()->service.complete(invalid,device,s,"fake-alice"));}
 @Test void logoutAfterAccessExpiresAndStorageContainsNoBearer(){var issued=login();assertEquals(0,jdbc.queryForObject("SELECT COUNT(*) FROM account_sessions WHERE access_hash=? OR refresh_hash=?",Integer.class,issued.get("access_token"),issued.get("refresh_token")));jdbc.update("UPDATE account_sessions SET access_expires=0");service.logoutRefresh(issued.get("refresh_token").toString(),device);assertThrows(IllegalArgumentException.class,()->service.refresh(issued.get("refresh_token").toString(),device));}
 @Test void fixturesAreIdempotentAndCannotBeRebound(){assertEquals(service.seed("alice","alice","alice@example.invalid"),service.seed("alice","alice","alice@example.invalid"));assertThrows(IllegalArgumentException.class,()->service.seed("alice","bruno","other@example.invalid"));}
 @Test void encryptionUsesDistinctNonceAndRejectsTampering(){String a=Crypto.encrypt("private-refresh"),b=Crypto.encrypt("private-refresh");assertNotEquals(a,b);assertEquals("private-refresh",Crypto.decrypt(a));assertThrows(RuntimeException.class,()->Crypto.decrypt(a.substring(0,a.length()-3)+"AAA"));}
}
