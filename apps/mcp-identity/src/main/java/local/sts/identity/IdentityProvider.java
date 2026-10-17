package local.sts.identity;
record ProviderTokens(String subject,String email,String refresh,long expiresAt) {}
interface IdentityProvider {
 String authorize(String state,String nonce,String challenge);
 ProviderTokens exchange(String code,String verifier,String nonce);
 ProviderTokens refresh(String refresh);
 void revoke(String refresh);
 String issuer();
}
