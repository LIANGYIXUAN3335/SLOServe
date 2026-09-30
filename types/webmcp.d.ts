type WebMCPTool = {
  name: string;
  title?: string;
  description: string;
  inputSchema: object;
  annotations?: {
    readOnlyHint?: boolean;
    untrustedContentHint?: boolean;
  };
  execute(input: unknown): unknown | Promise<unknown>;
};

type WebMCPModelContext = {
  registerTool(
    tool: WebMCPTool,
    options?: { signal?: AbortSignal },
  ): void | Promise<void>;
};

declare global {
  interface Document {
    readonly modelContext?: WebMCPModelContext;
  }
}

export {};
