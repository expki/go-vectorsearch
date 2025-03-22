export type ChatRequest = {
	prefix?: string;
	history?: string[];
	text: string;
	document_ids?: number[];
	documents?: any[];
};

export async function Chat(request: ChatRequest, setResponse: (result: string) => void, completed: () => void): Promise<void> {
  try {
    const response = await fetch('/api/chat', {
      method: 'POST',
      headers: {
        'Accept': 'text/plain',
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(request),
    });
    if (!response.body) {
      throw new Error("ReadableStream not supported in this browser.");
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();

    let result = "";
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      const chunk = decoder.decode(value, { stream: true });
      result += chunk;

      setResponse(result);
    }
  } catch (err) {
    console.error("Error starting chat stream:", err);
    setResponse(String(err));
  } finally {
    completed();
  }
}
