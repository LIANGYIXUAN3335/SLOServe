FROM vllm/vllm-openai:v0.10.2

ENTRYPOINT ["vllm", "serve", "Qwen/Qwen2.5-1.5B-Instruct", "--host", "0.0.0.0", "--port", "8080", "--enable-prefix-caching", "--max-model-len", "4096"]
