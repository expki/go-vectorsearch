export type UploadRequest = {
    owner: string,
    category: string,
    name?: string,
    external_id?: string,
    documents: Array<DocumentUpload>,
    no_update?: boolean,
};

export type DocumentUpload = {
	external_id?: string,
  document: any,
}

export type UploadResponse = {
	document_ids: Array<number>,
}

export async function Upload(request: UploadRequest): Promise<UploadResponse | undefined> {
  try {
    const response = await fetch('/api/upload', {
      method: 'POST',
      headers: {
        'Accept': 'application/json',
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(request),
    });
    if (!response.ok) {
      throw new Error(response.statusText);
    }

    return await response.json() as UploadResponse;
  } catch (err) {
    console.error("Error uploading:", err);
    return undefined;
  }
}
