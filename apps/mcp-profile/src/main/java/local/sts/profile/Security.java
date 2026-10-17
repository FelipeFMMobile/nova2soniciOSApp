package local.sts.profile;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import jakarta.servlet.*;
import jakarta.servlet.http.*;
import org.springframework.context.annotation.*;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.web.SecurityFilterChain;
import org.springframework.web.filter.OncePerRequestFilter;
import org.springframework.security.web.authentication.UsernamePasswordAuthenticationFilter;
@Configuration
class Security {
 @Bean SecurityFilterChain chain(HttpSecurity http) throws Exception {
  String secret=System.getenv("STS_SERVICE_TOKEN");
  if(secret==null||secret.length()<32) throw new IllegalStateException("STS_SERVICE_TOKEN must have at least 32 characters");
  return http.csrf(c->c.disable()).sessionManagement(s->s.sessionCreationPolicy(org.springframework.security.config.http.SessionCreationPolicy.STATELESS))
   .authorizeHttpRequests(a->a.anyRequest().permitAll()).addFilterBefore(new OncePerRequestFilter() {
    protected void doFilterInternal(HttpServletRequest r,HttpServletResponse w,FilterChain c) throws ServletException,IOException {
     w.setHeader("Cache-Control","no-store");
     if(r.getRequestURI().equals("/healthz")) {c.doFilter(r,w);return;}
     String got=r.getHeader("Authorization");
     if(got==null||!MessageDigest.isEqual(got.getBytes(StandardCharsets.UTF_8),("Bearer "+secret).getBytes(StandardCharsets.UTF_8))) {w.setStatus(401);return;}
     if(r.getContentLengthLong()>32768) {w.setStatus(413);return;}
     if(r.getMethod().equals("POST")) {
      byte[] bytes=r.getInputStream().readNBytes(32769);if(bytes.length>32768){w.setStatus(413);return;}
      try { var parser=Envelope.JSON.getFactory().createParser(bytes); if(Envelope.JSON.readTree(parser)==null||parser.nextToken()!=null){w.setStatus(400);return;} }
      catch(Exception invalid){w.setStatus(400);return;}
      c.doFilter(new HttpServletRequestWrapper(r) {
       public ServletInputStream getInputStream(){var stream=new java.io.ByteArrayInputStream(bytes);return new ServletInputStream(){
        public int read(){return stream.read();} public boolean isFinished(){return stream.available()==0;} public boolean isReady(){return true;} public void setReadListener(ReadListener listener){throw new UnsupportedOperationException();}
       };}
       public java.io.BufferedReader getReader(){return new java.io.BufferedReader(new java.io.InputStreamReader(getInputStream(),StandardCharsets.UTF_8));}
      },w);return;
     }
     c.doFilter(r,w);
    }
   },UsernamePasswordAuthenticationFilter.class).build();
 }
}
