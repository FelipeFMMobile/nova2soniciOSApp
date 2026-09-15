# AWS Bedrock setup

Nova 2 Sonic is a managed Bedrock model. This POC does not provision a GPU,
download weights, or expose an inference server.

## Account prerequisites

1. Enable access to `amazon.nova-2-sonic-v1:0` in `us-east-1`.
2. Attach the least-privilege policy in `iam-policy.json` to the development
   role or user.
3. Configure standard AWS credentials through AWS SSO or an AWS profile.
4. Do not use a Bedrock API key: bidirectional streaming requires standard AWS
   credentials and SigV4 authentication.

Recommended local configuration:

```bash
aws configure sso --profile sts-poc
export AWS_PROFILE=sts-poc
export AWS_REGION=us-east-1
aws sts get-caller-identity
```

The Python bridge uses the default AWS credential chain. Access keys must not
be placed in `.env`, source files, Xcode settings, or the application bundle.

## Start the local bridge

```bash
make nova-install
AWS_PROFILE=sts-poc make nova-bridge
```

The bridge binds to `127.0.0.1:8091` by default. It is an internal transport
adapter because the AWS bidirectional API is not currently exposed by the Go
SDK. The public gateway and all orchestration remain in Go.

## Live smoke test

Supply a short mono PCM16, 16 kHz WAV recording in Brazilian Portuguese:

```bash
AWS_PROFILE=sts-poc make nova-smoke WAV=/absolute/path/sample-pt-br.wav
```

The test opens a live Bedrock session and therefore incurs normal Bedrock
usage charges. It succeeds only after receiving audio output.

