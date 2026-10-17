package local.sts.identity;
import java.nio.charset.StandardCharsets;
import java.security.*;
import java.util.*;
import javax.crypto.*;
import javax.crypto.spec.*;
final class Crypto {
 static String random(){byte[] b=new byte[32];new SecureRandom().nextBytes(b);return Base64.getUrlEncoder().withoutPadding().encodeToString(b);}
 static String hash(String s){try{return Envelope.hex(MessageDigest.getInstance("SHA-256").digest(s.getBytes(StandardCharsets.UTF_8)));}catch(Exception e){throw new IllegalStateException();}}
 static String challenge(String s){try{return Base64.getUrlEncoder().withoutPadding().encodeToString(MessageDigest.getInstance("SHA-256").digest(s.getBytes(StandardCharsets.US_ASCII)));}catch(Exception e){throw new IllegalStateException();}}
 static byte[] key(String name){byte[] k=HexFormat.of().parseHex(required(name));if(k.length!=32)throw new IllegalStateException("invalid key configuration");return k;}
 static String required(String name){String s=System.getenv(name);if(s==null||s.isBlank())throw new IllegalStateException("missing "+name);return s;}
 static String code(String id){try{Mac m=Mac.getInstance("HmacSHA256");m.init(new SecretKeySpec(key("STS_CLIENT_CODE_KEY"),"HmacSHA256"));return Envelope.hex(m.doFinal(id.getBytes(StandardCharsets.UTF_8)));}catch(Exception e){throw new IllegalStateException("client code unavailable");}}
 static String encrypt(String plaintext){try{byte[] iv=new byte[12];new SecureRandom().nextBytes(iv);Cipher c=Cipher.getInstance("AES/GCM/NoPadding");c.init(Cipher.ENCRYPT_MODE,new SecretKeySpec(key("STS_TOKEN_ENCRYPTION_KEY"),"AES"),new GCMParameterSpec(128,iv));byte[] body=c.doFinal(plaintext.getBytes(StandardCharsets.UTF_8));return Base64.getEncoder().encodeToString(iv)+"."+Base64.getEncoder().encodeToString(body);}catch(Exception e){throw new IllegalStateException("encryption unavailable");}}
 static String decrypt(String encoded){try{String[] p=encoded.split("\\.");Cipher c=Cipher.getInstance("AES/GCM/NoPadding");c.init(Cipher.DECRYPT_MODE,new SecretKeySpec(key("STS_TOKEN_ENCRYPTION_KEY"),"AES"),new GCMParameterSpec(128,Base64.getDecoder().decode(p[0])));return new String(c.doFinal(Base64.getDecoder().decode(p[1])),StandardCharsets.UTF_8);}catch(Exception e){throw new IllegalArgumentException("invalid ciphertext");}}
}
