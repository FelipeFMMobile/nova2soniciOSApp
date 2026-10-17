package local.sts.profile;
import org.junit.jupiter.api.Test;
import static org.junit.jupiter.api.Assertions.*;
class ProfileValidationTest {
 @Test void preservesOptionalRemovalAndSelfDescribedGender() throws Exception {ProfileService.validate(Envelope.JSON.readTree("{\"name\":\"Maria\",\"age\":0,\"gender\":\"não binário\",\"city\":null,\"profession\":null}"));ProfileService.validate(Envelope.JSON.readTree("{\"age\":130}"));}
 @Test void rejectsIdentityAndInvalidFields() {for(String input:new String[]{"{}","{\"subject\":\"other\"}","{\"client_code\":\"other\"}","{\"name\":null}","{\"name\":\" \"}","{\"age\":131}","{\"age\":-1}","{\"age\":2.1}","{\"age\":\"2\"}","{\"gender\":\""+"x".repeat(81)+"\"}","{\"city\":\""+"x".repeat(121)+"\"}"})assertThrows(Exception.class,()->ProfileService.validate(Envelope.JSON.readTree(input)),input);}
}
