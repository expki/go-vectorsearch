export type SearchRequest = {
    owner: string,
    category: string,
    prefix?: string,
    text: string,
    count: number,
    offset?: number,
    centroids?: number,
};

export type SearchResponse = {
	documents: Array<DocumentSearch>,
}

export type DocumentSearch = {
	external_id?: string,
  document: any,
	document_id: number,
	document_similarity: number,
	centroid_similarity: number,
}

export async function Search(request: SearchRequest): Promise<SearchResponse | undefined> {
  try {
    const response = await fetch('/api/search', {
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

    return await response.json() as SearchResponse;
  } catch (err) {
    console.error("Error searching:", err);
    return undefined;
  }
}
