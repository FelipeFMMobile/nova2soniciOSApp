package local.sts.identity;
import com.fasterxml.jackson.databind.JsonNode;
import java.net.*;
import java.net.http.*;
import java.nio.charset.StandardCharsets;
import java.time.*;
import java.util.*;
import org.springframework.stereotype.Component;
import org.springframework.security.oauth2.jwt.*;

@Component
final class CognitoProvider implements IdentityProvider {
 private final HttpClient http;
 private final boolean fake;
 private final String domain,client,issuer,redirect;
 private final JwtDecoder decoder;
 CognitoProvider(){
  http=HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(5)).followRedirects(HttpClient.Redirect.NEVER).build();
  fake="fake".equals(System.getenv("STS_IDENTITY_PROVIDER"));
  if(fake){if(!"development".equals(System.getenv("STS_ENV")))throw new IllegalStateException("fake identity requires development");domain="";client="";issuer="urn:sts:fake";redirect="stsvoice://oauth/callback";decoder=null;return;}
  domain=Crypto.required("STS_COGNITO_DOMAIN"); client=Crypto.required("STS_COGNITO_CLIENT_ID");issuer=Crypto.required("STS_COGNITO_ISSUER");redirect=Crypto.required("STS_COGNITO_REDIRECT_URI");
  if(!domain.matches("https://[A-Za-z0-9.-]+")||!issuer.matches("https://cognito-idp\\.[a-z0-9-]+\\.amazonaws\\.com/[A-Za-z0-9_-]+")||!redirect.equals("stsvoice://oauth/callback"))throw new IllegalStateException("invalid Cognito endpoints");
  NimbusJwtDecoder d=NimbusJwtDecoder.withJwkSetUri(issuer+"/.well-known/jwks.json").build();d.setJwtValidator(JwtValidators.createDefaultWithIssuer(issuer));decoder=d;
 }
 // Package-local injection exercises the real adapter without network or AWS credentials.
 CognitoProvider(String domain,String client,String issuer,String redirect,HttpClient http,JwtDecoder decoder){
  this.fake=false;this.domain=domain;this.client=client;this.issuer=issuer;this.redirect=redirect;this.http=http;this.decoder=decoder;
 }
 public String issuer(){return issuer;}
 private static String enc(String v){return URLEncoder.encode(v,StandardCharsets.UTF_8);}
 public String authorize(String state,String nonce,String challenge){
  if(fake)return "stsvoice://oauth/callback?code=fake-alice&state="+enc(state);
  return domain+"/oauth2/authorize?response_type=code&client_id="+enc(client)+"&redirect_uri="+enc(redirect)+"&scope="+enc("openid email aws.cognito.signin.user.admin")+"&code_challenge_method=S256&code_challenge="+enc(challenge)+"&state="+enc(state)+"&nonce="+enc(nonce)+"&prompt=login";
 }
 private JsonNode post(String endpoint,String contentType,String body,String target){
  try{HttpRequest.Builder b=HttpRequest.newBuilder(URI.create(endpoint)).timeout(Duration.ofSeconds(10)).header("Content-Type",contentType).POST(HttpRequest.BodyPublishers.ofString(body));if(target!=null)b.header("X-Amz-Target",target);
   HttpResponse<String> r=http.send(b.build(),HttpResponse.BodyHandlers.ofString());if(r.statusCode()!=200||r.body().length()>32768)throw new IllegalArgumentException();return Envelope.JSON.readTree(r.body());
  }catch(Exception e){throw new IllegalArgumentException("identity_provider_unavailable");}
 }
 private ProviderTokens validate(JsonNode data,String nonce,String previousRefresh){
  Jwt access=decoder.decode(data.path("access_token").asText());
  if(!"access".equals(access.getClaimAsString("token_use"))||!client.equals(access.getClaimAsString("client_id")))Envelope.deny();
  String subject=access.getSubject();if(subject==null||access.getExpiresAt()==null)Envelope.deny();
  if(nonce!=null){Jwt id=decoder.decode(data.path("id_token").asText());if(id.getExpiresAt()==null||!"id".equals(id.getClaimAsString("token_use"))||!id.getAudience().contains(client)||!subject.equals(id.getSubject())||!nonce.equals(id.getClaimAsString("nonce")))Envelope.deny();}
  // Cognito JWTs do not assert that email MFA occurred. Restrict admission to
  // provisioned users with EMAIL_OTP enabled; make dev also verifies REQUIRED MFA.
  JsonNode user=post(issuer.substring(0,issuer.lastIndexOf('/')),"application/x-amz-json-1.1", "{\"AccessToken\":"+Envelope.quote(data.path("access_token").asText())+"}","AWSCognitoIdentityProviderService.GetUser");
  boolean mfa=false;for(JsonNode factor:user.path("UserMFASettingList"))mfa|="EMAIL_OTP".equals(factor.asText());if(!mfa)Envelope.deny();
  String email="",verified="";for(JsonNode a:user.path("UserAttributes")){if(a.path("Name").asText().equals("email"))email=a.path("Value").asText();if(a.path("Name").asText().equals("email_verified"))verified=a.path("Value").asText();}
  if(email.isBlank()||!verified.equals("true"))Envelope.deny();
  String refresh=data.path("refresh_token").asText(previousRefresh);if(refresh==null||refresh.isBlank())Envelope.deny();return new ProviderTokens(subject,email,refresh,access.getExpiresAt().getEpochSecond());
 }
 public ProviderTokens exchange(String code,String verifier,String nonce){
  if(fake){if(!code.matches("fake-(alice|bruno|carla)"))Envelope.deny();return new ProviderTokens(code.substring(5),code.substring(5)+"@example.invalid","fake:"+code.substring(5),Instant.now().getEpochSecond()+900);}
  return validate(post(domain+"/oauth2/token","application/x-www-form-urlencoded","grant_type=authorization_code&client_id="+enc(client)+"&code="+enc(code)+"&redirect_uri="+enc(redirect)+"&code_verifier="+enc(verifier),null),nonce,null);
 }
 public ProviderTokens refresh(String token){if(fake){if(!token.matches("fake:(alice|bruno|carla)"))Envelope.deny();String s=token.substring(5);return new ProviderTokens(s,s+"@example.invalid",token,Instant.now().getEpochSecond()+900);}return validate(post(domain+"/oauth2/token","application/x-www-form-urlencoded","grant_type=refresh_token&client_id="+enc(client)+"&refresh_token="+enc(token),null),null,token);}
 public void revoke(String token){if(fake)return;try{http.send(HttpRequest.newBuilder(URI.create(domain+"/oauth2/revoke")).timeout(Duration.ofSeconds(5)).header("Content-Type","application/x-www-form-urlencoded").POST(HttpRequest.BodyPublishers.ofString("client_id="+enc(client)+"&token="+enc(token))).build(),HttpResponse.BodyHandlers.discarding());}catch(Exception ignored){/* Local revocation remains authoritative. */}}
}
