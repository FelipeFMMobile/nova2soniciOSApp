package local.sts.profile;
import org.springframework.web.bind.annotation.*;
import java.util.Map;
@RestController
class ProfileController {
 private final ProfileService service;ProfileController(ProfileService service){this.service=service;}
 record Fixture(String client_code,String name){}
 @PostMapping("/internal/fixtures") Map<String,Object> fixture(@RequestBody Fixture f){return service.seed(f.client_code(),f.name());}
}
