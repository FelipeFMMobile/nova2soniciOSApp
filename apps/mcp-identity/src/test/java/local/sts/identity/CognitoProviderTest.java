package local.sts.identity;

import com.nimbusds.jose.*;
import com.nimbusds.jose.crypto.RSASSASigner;
import com.nimbusds.jwt.*;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.net.*;
import java.net.http.*;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.security.*;
import java.security.interfaces.*;
import java.time.Instant;
import java.time.Duration;
import javax.net.ssl.*;
import java.util.*;
import java.util.concurrent.*;
import java.util.concurrent.Flow;
import org.junit.jupiter.api.*;
import org.springframework.security.oauth2.jwt.*;
import static org.junit.jupiter.api.Assertions.*;

class CognitoProviderTest {
 static final String ISSUER="https://cognito-idp.us-east-1.amazonaws.com/us-east-1_local";
 static final String DOMAIN="https://local.auth.us-east-1.amazoncognito.com";
 static final String CLIENT="test-client", REDIRECT="stsvoice://oauth/callback", NONCE="test-nonce";
 static KeyPair key,otherKey;
 TestHttpClient http;
 CognitoProvider provider;
 final List<HttpRequest> requests=new ArrayList<>();
 Map<String,Object> accessClaims,idClaims;
 String tokenResponse,userResponse;
 int tokenStatus;

 @BeforeAll static void keys() throws Exception {
  KeyPairGenerator generator=KeyPairGenerator.getInstance("RSA");generator.initialize(2048);
  key=generator.generateKeyPair();otherKey=generator.generateKeyPair();
 }
 @BeforeEach void setup() throws Exception {
  accessClaims=new HashMap<>(Map.of("token_use","access","client_id",CLIENT));
  idClaims=new HashMap<>(Map.of("token_use","id","aud",List.of(CLIENT),"nonce",NONCE));
  tokenStatus=200;
  userResponse="{\"UserMFASettingList\":[\"EMAIL_OTP\"],\"UserAttributes\":[{\"Name\":\"email\",\"Value\":\"alice@example.invalid\"},{\"Name\":\"email_verified\",\"Value\":\"true\"}]}";
  tokens();
  http=new TestHttpClient();
  NimbusJwtDecoder decoder=NimbusJwtDecoder.withPublicKey((RSAPublicKey)key.getPublic()).build();
  decoder.setJwtValidator(JwtValidators.createDefaultWithIssuer(ISSUER));
  provider=new CognitoProvider(DOMAIN,CLIENT,ISSUER,REDIRECT,http,decoder);
 }
 // No sockets or JVM agent: requests run through production serialization, while
 // JWT validation still uses Spring/Nimbus with real RSA signatures.
 class TestHttpClient extends HttpClient {
  boolean unavailable;
  final HttpClient settings=HttpClient.newHttpClient();
  public Optional<CookieHandler> cookieHandler(){return Optional.empty();}
  public Optional<Duration> connectTimeout(){return Optional.of(Duration.ofSeconds(5));}
  public Redirect followRedirects(){return Redirect.NEVER;}
  public Optional<ProxySelector> proxy(){return Optional.empty();}
  public SSLContext sslContext(){return settings.sslContext();}
  public SSLParameters sslParameters(){return settings.sslParameters();}
  public Optional<Authenticator> authenticator(){return Optional.empty();}
  public Version version(){return Version.HTTP_1_1;}
  public Optional<Executor> executor(){return Optional.empty();}
  @SuppressWarnings("unchecked")
  public <T> HttpResponse<T> send(HttpRequest request,HttpResponse.BodyHandler<T> handler) throws IOException {
   if(unavailable)throw new IOException("private-token");
   requests.add(request);boolean token=request.uri().getPath().equals("/oauth2/token");
   return new HttpResponse<T>(){
    public int statusCode(){return token?tokenStatus:200;}
    public HttpRequest request(){return request;}
    public Optional<HttpResponse<T>> previousResponse(){return Optional.empty();}
    public HttpHeaders headers(){return HttpHeaders.of(Map.of(),(a,b)->true);}
    public T body(){return (T)(token?tokenResponse:userResponse);}
    public Optional<SSLSession> sslSession(){return Optional.empty();}
    public URI uri(){return request.uri();}
    public Version version(){return Version.HTTP_1_1;}
   };
  }
  public <T> CompletableFuture<HttpResponse<T>> sendAsync(HttpRequest request,HttpResponse.BodyHandler<T> handler){throw new UnsupportedOperationException();}
  public <T> CompletableFuture<HttpResponse<T>> sendAsync(HttpRequest request,HttpResponse.BodyHandler<T> handler,HttpResponse.PushPromiseHandler<T> push){throw new UnsupportedOperationException();}
 }
 String jwt(Map<String,Object> claims,KeyPair signingKey) throws Exception {
  Instant now=Instant.now();
  JWTClaimsSet.Builder builder=new JWTClaimsSet.Builder().issuer(ISSUER).subject("alice")
   .issueTime(Date.from(now.minusSeconds(5))).expirationTime(Date.from(now.plusSeconds(900)));
  claims.forEach(builder::claim);
  SignedJWT signed=new SignedJWT(new JWSHeader(JWSAlgorithm.RS256),builder.build());
  signed.sign(new RSASSASigner((RSAPrivateKey)signingKey.getPrivate()));return signed.serialize();
 }
 void tokens() throws Exception {
  tokenResponse=Envelope.JSON.writeValueAsString(Map.of("access_token",jwt(accessClaims,key),
   "id_token",jwt(idClaims,key),"refresh_token","test-refresh"));
 }
 static Map<String,String> form(String query){
  Map<String,String> result=new HashMap<>();
  for(String item:query.split("&")){String[] pair=item.split("=",2);result.put(URLDecoder.decode(pair[0],StandardCharsets.UTF_8),URLDecoder.decode(pair[1],StandardCharsets.UTF_8));}
  return result;
 }
 static String body(HttpRequest request) throws Exception {
  var result=new CompletableFuture<String>();var bytes=new ByteArrayOutputStream();
  request.bodyPublisher().orElseThrow().subscribe(new Flow.Subscriber<ByteBuffer>(){
   public void onSubscribe(Flow.Subscription subscription){subscription.request(Long.MAX_VALUE);}
   public void onNext(ByteBuffer buffer){byte[] part=new byte[buffer.remaining()];buffer.get(part);bytes.writeBytes(part);}
   public void onError(Throwable failure){result.completeExceptionally(failure);}
   public void onComplete(){result.complete(bytes.toString(StandardCharsets.UTF_8));}
  });
  return result.get(2,TimeUnit.SECONDS);
 }
 void deniedBeforeUserLookup() {
  assertThrows(RuntimeException.class,()->provider.exchange("code","verifier",NONCE));
  assertEquals(1,requests.size(),"Invalid JWT must not reach GetUser");requests.clear();
 }

 @Test void authorizeBindsPkceStateNonceAndCallback(){
  URI uri=URI.create(provider.authorize("state +&","nonce /&","challenge_-"));
  assertEquals(DOMAIN+"/oauth2/authorize",uri.getScheme()+"://"+uri.getHost()+uri.getPath());
  assertEquals(Map.of("response_type","code","client_id",CLIENT,"redirect_uri",REDIRECT,
   "scope","openid email aws.cognito.signin.user.admin","code_challenge_method","S256",
   "code_challenge","challenge_-","state","state +&","nonce","nonce /&","prompt","login"),form(uri.getRawQuery()));
 }
 @Test void exchangeValidatesSignedTokensAndSendsPkceVerifier() throws Exception {
  ProviderTokens result=provider.exchange("code +&","verifier /&",NONCE);
  assertEquals("alice",result.subject());assertEquals("alice@example.invalid",result.email());
  assertEquals("test-refresh",result.refresh());assertTrue(result.expiresAt()>Instant.now().getEpochSecond());
  assertEquals(Map.of("grant_type","authorization_code","client_id",CLIENT,"code","code +&",
   "redirect_uri",REDIRECT,"code_verifier","verifier /&"),form(body(requests.getFirst())));
  HttpRequest user=requests.get(1);
  assertEquals(URI.create("https://cognito-idp.us-east-1.amazonaws.com"),user.uri());
  assertEquals("AWSCognitoIdentityProviderService.GetUser",user.headers().firstValue("X-Amz-Target").orElseThrow());
  assertEquals(Envelope.JSON.readTree(tokenResponse).get("access_token"),Envelope.JSON.readTree(body(user)).get("AccessToken"));
 }
 @Test void rejectsWrongSignatureIssuerExpiryAndAccessPurpose() throws Exception {
  tokenResponse=Envelope.JSON.writeValueAsString(Map.of("access_token",jwt(accessClaims,otherKey)));deniedBeforeUserLookup();
  for(var invalid:List.of(Map.of("iss","https://wrong.example.invalid"),Map.of("exp",0),
    Map.of("token_use","id"),Map.of("client_id","other-client"))){
   var original=new HashMap<>(accessClaims);accessClaims.putAll(invalid);tokens();deniedBeforeUserLookup();accessClaims=original;
  }
 }
 @Test void rejectsWrongNonceAudienceSubjectAndIdPurpose() throws Exception {
  for(var invalid:List.of(Map.of("nonce","wrong"),Map.of("aud",List.of("other-client")),
    Map.of("sub","bruno"),Map.of("token_use","access"))){
   var original=new HashMap<>(idClaims);idClaims.putAll(invalid);tokens();deniedBeforeUserLookup();idClaims=original;
  }
 }
 @Test void requiresExpiryOnBothTokens() throws Exception {
  accessClaims.put("exp",null);tokens();deniedBeforeUserLookup();
  accessClaims.remove("exp");idClaims.put("exp",null);tokens();deniedBeforeUserLookup();
 }
 @Test void requiresEmailMfaAndVerifiedEmail(){
  for(String invalid:List.of(userResponse.replace("EMAIL_OTP","SMS_MFA"),
    userResponse.replace("\"true\"","\"false\""),userResponse.replace("alice@example.invalid",""))){
   String original=userResponse;userResponse=invalid;
   assertThrows(IllegalArgumentException.class,()->provider.exchange("code","verifier",NONCE));
   userResponse=original;
  }
 }
 @Test void refreshRetainsOrRotatesProviderCredentialWithoutReusingNonce() throws Exception {
  String access=jwt(accessClaims,key);
  tokenResponse=Envelope.JSON.writeValueAsString(Map.of("access_token",access));
  assertEquals("old-refresh",provider.refresh("old-refresh").refresh());
  assertEquals(Map.of("grant_type","refresh_token","client_id",CLIENT,"refresh_token","old-refresh"),form(body(requests.getFirst())));
  tokenResponse=Envelope.JSON.writeValueAsString(Map.of("access_token",access,"refresh_token","rotated-refresh"));
  assertEquals("rotated-refresh",provider.refresh("old-refresh").refresh());
 }
 @Test void providerFailuresAreClosedAndDoNotExposeResponseContent() throws Exception {
  for(String invalid:List.of("private-otp-and-token", "{", "x".repeat(32769))){
   tokenResponse=invalid;
   assertEquals("identity_provider_unavailable",assertThrows(IllegalArgumentException.class,
    ()->provider.exchange("code","verifier",NONCE)).getMessage());
  }
  tokens();tokenStatus=503;
  assertEquals("identity_provider_unavailable",assertThrows(IllegalArgumentException.class,
   ()->provider.exchange("code","verifier",NONCE)).getMessage());
  http.unavailable=true;
  assertEquals("identity_provider_unavailable",assertThrows(IllegalArgumentException.class,
   ()->provider.refresh("private-refresh")).getMessage());
 }
 @Test void revokeSendsOnlyDedicatedControlRequestAndToleratesUnavailableProvider() throws Exception {
  provider.revoke("refresh +&");
  assertEquals(URI.create(DOMAIN+"/oauth2/revoke"),requests.getFirst().uri());
  assertEquals(Map.of("client_id",CLIENT,"token","refresh +&"),form(body(requests.getFirst())));
  http.unavailable=true;
  assertDoesNotThrow(()->provider.revoke("refresh"));
 }
}
