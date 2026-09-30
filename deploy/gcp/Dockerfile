FROM vllm/vllm-openai@sha256:607442e407b0fea97f8a132a78b787c121a996dd4de181fa08e8da06e71ec2db

ENTRYPOINT ["vllm", "serve", "Qwen/Qwen2.5-1.5B-Instruct", "--revision", "989aa7980e4cf806f80c7fef2b1adb7bc71aa306", "--host", "0.0.0.0", "--port", "8080", "--enable-prefix-caching", "--max-model-len", "4096"]
