package local.sts.identity;
import java.util.Map;
import org.springframework.web.bind.annotation.*;
import org.springframework.http.ResponseEntity;
@RestControllerAdvice
class Errors {
 @ExceptionHandler(Exception.class) ResponseEntity<Map<String,String>> failed(Exception e) {
  return ResponseEntity.status(e instanceof IllegalArgumentException?400:503).body(Map.of("error","operation_rejected"));
 }
}
@RestController
class Health { @GetMapping("/healthz") Map<String,String> health() {return Map.of("status","ok");} }
