export interface EagleImagePayload {
  url: string;
  name?: string;
}

export interface EagleSaveImagesPayload {
  images: EagleImagePayload[];
  article_title?: string;
  article_url?: string;
  feed_title?: string;
  feed_url?: string;
}

export interface EagleSaveImagesResult {
  success: number;
  failed: number;
  total: number;
  errors?: string[];
}

export async function saveImagesToEagle(
  payload: EagleSaveImagesPayload
): Promise<EagleSaveImagesResult> {
  const response = await fetch('/api/eagle/save-images', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });

  const text = await response.text();
  let data: EagleSaveImagesResult | null = null;
  if (text) {
    try {
      data = JSON.parse(text) as EagleSaveImagesResult;
    } catch {
      // Keep the original text for the error below.
    }
  }

  if (!response.ok) {
    const message = data?.errors?.[0] || text || 'Failed to save to Eagle';
    throw new Error(message);
  }

  if (!data) {
    throw new Error('Invalid Eagle response');
  }

  return data;
}
