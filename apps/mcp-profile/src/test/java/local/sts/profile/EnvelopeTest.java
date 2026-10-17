package local.sts.profile;
import org.junit.jupiter.api.Test;
import static org.junit.jupiter.api.Assertions.*;
import java.util.*;
class EnvelopeTest {
 @Test void consumesGoVectorAndRejectsTampering() throws Exception {
  var input=Envelope.JSON.readValue(getClass().getResourceAsStream("/contracts/v2/envelope-vector.json"),Map.class);
  var verified=Envelope.verifyAt("profile","profile.update_me",input,2000000000L);
  assertEquals("Á <&>",verified.payload().path("changes").path("name").asText());
  assertThrows(IllegalArgumentException.class,()->Envelope.verifyAt("identity","profile.update_me",input,2000000000L));
  assertThrows(IllegalArgumentException.class,()->Envelope.verifyAt("profile","profile.get_me",input,2000000000L));
  assertThrows(IllegalArgumentException.class,()->Envelope.verifyAt("profile","profile.update_me",input,2000000061L));
  ((Map)((Map)input.get("host_context")).get("identity")).put("subject","bruno");
  assertThrows(IllegalArgumentException.class,()->Envelope.verifyAt("profile","profile.update_me",input,2000000000L));
 }
 @Test void preservesSignedNullFields() throws Exception {
  var input=Envelope.JSON.readValue(getClass().getResourceAsStream("/contracts/v2/envelope-null-vector.json"),Map.class);
  var verified=Envelope.verifyAt("profile","profile.update_me",input,2000000000L);
  assertTrue(verified.payload().path("changes").has("gender"));
  assertTrue(verified.payload().path("changes").get("gender").isNull());
 }
 @Test void canonicalStringsMatchGo() throws Exception {assertEquals("{\"name\":\"Á \\u003c\\u0026\\u003e\"}",Envelope.canonical(Envelope.JSON.readTree("{\"name\":\"Á <&>\"}")));}
}
