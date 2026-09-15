# RunPod Qwen deployment

This directory describes the billable, remote part of the POC. Provisioning is
intentionally not automatic: creating a Pod starts external charges.

## 1. Create persistent storage

In the RunPod console, create a 200 GB Secure Cloud network volume named
`sts-qwen-models` in `US-GA-2`. The desired API payload is captured in
`network-volume.template.json`.

Record the returned volume ID in a local, ignored copy of the Pod template:

```bash
cp deploy/runpod/pod.template.json deploy/runpod/pod.local.json
```

Replace `REPLACE_WITH_NETWORK_VOLUME_ID` only in `pod.local.json`.

## 2. Build and publish the image

Build on an NVIDIA-capable Linux host or through the RunPod registry workflow:

```bash
docker build -t YOUR_REGISTRY/sts-qwen-omni:0.2.0 deploy/runpod
docker push YOUR_REGISTRY/sts-qwen-omni:0.2.0
```

Configure the Pod to use that immutable image tag or, preferably, its digest.

## 3. Create the Pod

Create one Secure Cloud Pod with:

- datacenter `US-GA-2`;
- one NVIDIA H200;
- the previously created network volume mounted at `/workspace`;
- 50 GB container disk;
- port `8091/http` exposed;
- a random `VLLM_API_KEY` of at least 24 characters.

Do not place a real secret into either JSON template. Set secrets in the RunPod
console or API request assembled outside this repository.

## 4. Wait for model readiness

The first boot downloads the pinned model revision into `/workspace/models` and
can take several minutes. After the service is ready, configure the gateway:

```bash
export STS_PROVIDER=qwen
export STS_QWEN_REALTIME_URL=wss://POD_ID-8091.proxy.runpod.net/v1/realtime
export STS_QWEN_CHAT_URL=https://POD_ID-8091.proxy.runpod.net/v1/chat/completions
export STS_QWEN_API_KEY='value-from-password-manager'
```

## 5. Smoke test

Create a Python 3.12 virtual environment outside tracked source files and
install `websockets`. Supply a short Brazilian Portuguese WAV file that is
mono PCM16 at 16 kHz:

```bash
python tests/e2e/qwen_realtime_smoke.py sample-pt-br.wav
```

The test succeeds only when response audio is received. The JSON it prints may
include a transcript but never includes credentials.

## 6. Stop compute

Stop the Pod immediately after development. Do not delete the network volume;
it owns the model cache. Check the RunPod billing page after every session.

## Known gate

Repository checks validate the container, templates, and client code without
spending money. The Stage 01 runtime gate remains pending until an authorized
RunPod account creates the volume and Pod and the live smoke test passes.

