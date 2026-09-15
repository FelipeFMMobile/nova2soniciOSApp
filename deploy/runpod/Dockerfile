ARG VLLM_OMNI_IMAGE=vllm/vllm-omni:v0.28.0
FROM ${VLLM_OMNI_IMAGE}

COPY start-qwen.sh /usr/local/bin/start-qwen
RUN chmod 0755 /usr/local/bin/start-qwen

EXPOSE 8091
ENTRYPOINT ["/usr/local/bin/start-qwen"]

