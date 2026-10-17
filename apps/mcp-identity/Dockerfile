FROM maven:3.9.9-eclipse-temurin-21 AS build
WORKDIR /app
COPY apps/mcp-identity/pom.xml .
COPY apps/mcp-identity/src src
COPY apps/mcp-identity/contracts contracts
RUN mvn -B package -DskipTests
FROM eclipse-temurin:21-jre
WORKDIR /app
COPY --from=build /app/target/mcp-identity-0.1.0.jar app.jar
USER 10001:10001
ENTRYPOINT ["java", "-jar", "/app/app.jar"]
