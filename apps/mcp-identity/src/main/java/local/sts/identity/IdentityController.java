package local.sts.identity;
import java.util.Map;
import org.springframework.web.bind.annotation.*;
@RestController
class IdentityController {
 private final IdentityService service;
 IdentityController(IdentityService service){this.service=service;}
 record Begin(String device_id){}
 record Prepare(String transaction_id,String device_id){}
 record Complete(String transaction_id,String device_id,String state,String code){}
 record Refresh(String refresh_token,String device_id){}
 record Access(String access_token,String device_id){}
 record Session(String session_ref,String subject,String client_code,String device_id){}
 record Seed(String fixture,String subject,String email){}
 @PostMapping("/auth/begin") Map<String,Object> begin(@RequestBody Begin b){return Map.of("transaction_id",service.begin(b.device_id()));}
 @PostMapping("/auth/prepare") Map<String,Object> prepare(@RequestBody Prepare b){return service.prepare(b.transaction_id(),b.device_id());}
 @PostMapping("/auth/complete") Map<String,Object> complete(@RequestBody Complete b){return service.complete(b.transaction_id(),b.device_id(),b.state(),b.code());}
 @PostMapping("/auth/refresh") Map<String,Object> refresh(@RequestBody Refresh b){return service.refresh(b.refresh_token(),b.device_id());}
 @PostMapping("/auth/logout") Map<String,Object> logout(@RequestBody Refresh b){service.logoutRefresh(b.refresh_token(),b.device_id());return Map.of("status","signed_out");}
 @PostMapping("/internal/introspect") Map<String,Object> introspect(@RequestBody Access b){return service.introspect(b.access_token(),b.device_id());}
 @PostMapping("/internal/session") Map<String,Object> session(@RequestBody Session b){return service.session(b.session_ref(),b.subject(),b.client_code(),b.device_id());}
 @PostMapping("/internal/fixtures") Map<String,Object> seed(@RequestBody Seed b){return service.seed(b.fixture(),b.subject(),b.email());}
}
