export type FetchCategoryNamesRequest = {
	owner: string,
}

export type FetchCategoryNamesResponse = {
	category_names: Array<string>,
}

export async function GetCategories(request: FetchCategoryNamesRequest): Promise<FetchCategoryNamesResponse | undefined> {
  try {
    const response = await fetch('/api/categories', {
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

    return await response.json() as FetchCategoryNamesResponse;
  } catch (err) {
    console.error("Error getting categories:", err);
    return undefined;
  }
}
