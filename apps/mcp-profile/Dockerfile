FROM maven:3.9.9-eclipse-temurin-21 AS build
WORKDIR /app
COPY apps/mcp-profile/pom.xml .
COPY apps/mcp-profile/src src
COPY apps/mcp-profile/contracts contracts
RUN mvn -B package -DskipTests
FROM eclipse-temurin:21-jre
WORKDIR /app
COPY --from=build /app/target/mcp-profile-0.1.0.jar app.jar
USER 10001:10001
ENTRYPOINT ["java", "-jar", "/app/app.jar"]
