export async function DeleteOwner(owner: string): Promise<void> {
  try {
    const response = await fetch('/api/delete/owner', {
      method: 'POST',
      headers: {
        'Accept': 'application/json',
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        owner: owner,
      }),
    });
    if (!response.ok) {
      throw new Error(response.statusText);
    }

    await response.json();
  } catch (err) {
    console.error("Error deleting owner:", err);
  }
}

export async function DeleteCategory(owner: string, category: string): Promise<void> {
  try {
    const response = await fetch('/api/delete/category', {
      method: 'POST',
      headers: {
        'Accept': 'application/json',
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        owner: owner,
        category: category,
      }),
    });
    if (!response.ok) {
      throw new Error(response.statusText);
    }

    await response.json();
  } catch (err) {
    console.error("Error deleting category:", err);
  }
}

export async function DeleteDocument(owner: string, category: string, documentID: number): Promise<void> {
  try {
    const response = await fetch('/api/delete/category', {
      method: 'POST',
      headers: {
        'Accept': 'application/json',
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        owner: owner,
        category: category,
        document_id: documentID,
      }),
    });
    if (!response.ok) {
      throw new Error(response.statusText);
    }

    await response.json();
  } catch (err) {
    console.error("Error deleting category:", err);
  }
}
