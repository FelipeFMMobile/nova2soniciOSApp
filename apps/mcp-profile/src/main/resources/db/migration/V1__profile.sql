CREATE TABLE profiles (
 client_code VARCHAR(64) PRIMARY KEY,
 name VARCHAR(120) NOT NULL,
 age INTEGER CHECK(age BETWEEN 0 AND 130),
 city VARCHAR(120),
 gender VARCHAR(80),
 profession VARCHAR(120),
 version BIGINT NOT NULL DEFAULT 1
);
CREATE TABLE profile_operations (
 client_code VARCHAR(64) NOT NULL REFERENCES profiles(client_code),
 operation_key VARCHAR(256) NOT NULL,
 payload_hash VARCHAR(64) NOT NULL,
 result_json TEXT NOT NULL,
 PRIMARY KEY(client_code,operation_key)
);
