/**
 * Media proxy utilities for handling anti-hotlinking and caching
 */

// Cache for media cache enabled setting to avoid repeated API calls
let mediaCacheEnabledCache: boolean | null = null;
let mediaCachePromise: Promise<boolean> | null = null;

/**
 * Convert a media URL to use the proxy endpoint
 * @param url Original media URL
 * @param referer Optional referer URL for anti-hotlinking and resolving relative URLs
 * @returns Proxied URL
 */
export function getProxiedMediaUrl(url: string, referer?: string): string {
  if (!url) return '';

  // Don't proxy data URLs or blob URLs
  if (url.startsWith('data:') || url.startsWith('blob:')) {
    return url;
  }

  // Unwrap jintiankansha wrapper URLs for better proxy success
  const unwrappedUrl = unwrapJintiankanshaUrl(url);

  // Don't proxy Douban blog images (hotlink protection causes proxy failures)
  if (shouldBypassProxy(unwrappedUrl)) {
    return unwrappedUrl;
  }

  // Don't proxy localhost URLs (these are local development URLs)
  if (url.startsWith('http://localhost') || url.startsWith('http://127.0.0.1')) {
    return url;
  }

  // Resolve relative URLs using the referer
  // Relative URLs need to be converted to absolute URLs before proxying
  let urlToProxy = unwrappedUrl;
  if (referer && !unwrappedUrl.startsWith('http://') && !unwrappedUrl.startsWith('https://')) {
    // This is a relative URL, resolve it against the referer
    try {
      const baseUrl = new URL(referer);
      urlToProxy = new URL(unwrappedUrl, baseUrl).href;
    } catch (e) {
      // If URL resolution fails, use the original URL
      console.warn('Failed to resolve relative URL:', unwrappedUrl, 'against referer:', referer, e);
      urlToProxy = unwrappedUrl;
    }
  }

  // CRITICAL FIX: Use base64 encoding to avoid all URL encoding issues
  // This prevents double-encoding problems with special characters, Chinese characters, etc.
  // Base64 encoding is safe for URLs and doesn't interfere with query parameter parsing
  let urlB64 = '';
  try {
    urlB64 = encodeToBase64(urlToProxy);
  } catch (error) {
    console.warn('Failed to base64-encode media URL:', urlToProxy, error);
    return urlToProxy;
  }

  // Build proxy URL with base64-encoded parameters
  let proxyUrl = `/api/media/proxy?url_b64=${urlB64}`;

  // Add referer if provided (also base64-encoded)
  if (referer) {
    try {
      const refererB64 = encodeToBase64(referer);
      proxyUrl += `&referer_b64=${refererB64}`;
    } catch (error) {
      console.warn('Failed to base64-encode media referer:', referer, error);
      return urlToProxy;
    }
  }

  return proxyUrl;
}

function shouldBypassProxy(url: string): boolean {
  try {
    const hostname = new URL(url).hostname.toLowerCase();
    return (
      hostname === '500px.me' ||
      hostname.endsWith('.500px.me') ||
      hostname.endsWith('500px.com')|| hostname.includes("blog.douban.com")
    );
  } catch {
    return false;
  }
}

function encodeToBase64(value: string): string {
  if (typeof TextEncoder === 'undefined') {
    return btoa(value);
  }
  const bytes = new TextEncoder().encode(value);
  let binary = '';
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary);
}

/**
 * Check if media caching is enabled (with caching to avoid repeated API calls)
 * @returns Promise<boolean>
 */
export async function isMediaCacheEnabled(): Promise<boolean> {
  // Return cached value if available
  if (mediaCacheEnabledCache !== null) {
    return mediaCacheEnabledCache;
  }

  // If a request is already in flight, wait for it
  if (mediaCachePromise) {
    return mediaCachePromise;
  }

  // Start a new request
  mediaCachePromise = (async () => {
    try {
      const response = await fetch('/api/settings');
      if (response.ok) {
        const settings = await response.json();
        mediaCacheEnabledCache =
          settings.media_cache_enabled === 'true' || settings.media_cache_enabled === true;
        return mediaCacheEnabledCache;
      }
    } catch (error) {
      console.error('Failed to check media cache status:', error);
    }
    mediaCacheEnabledCache = false;
    return false;
  })();

  const result = await mediaCachePromise;
  mediaCachePromise = null; // Clear the promise after completion
  return result;
}

/**
 * Clear the media cache enabled cache (call this when settings change)
 */
export function clearMediaCacheEnabledCache(): void {
  mediaCacheEnabledCache = null;
}

/**
 * Process HTML content to proxy media URLs (images, videos, sources)
 * @param html HTML content
 * @param referer Optional referer URL
 * @returns HTML with proxied media URLs
 * @note Unquoted src attributes are supported but must not contain spaces (per HTML spec)
 */
export function proxyImagesInHtml(html: string, referer?: string): string {
  if (!html) return html;

  let processed = unwrapJintiankanshaInHtml(html);

  // First, convert lazy-loaded images to normal images
  // This ensures images load immediately without waiting for lazy loading scripts
  processed = convertLazyImages(processed);

  // Proxy image src attributes
  processed = proxyElementAttribute(processed, 'img', 'src', referer, true);

  // Proxy video src and poster attributes
  processed = proxyElementAttribute(processed, 'video', 'src', referer);
  processed = proxyElementAttribute(processed, 'video', 'poster', referer);

  // Proxy source src attributes (inside video/audio tags)
  processed = proxyElementAttribute(processed, 'source', 'src', referer);

  return processed;
}

/**
 * Convert lazy-loaded images to normal images
 * For images with data-original or data-src attributes, move those URLs to src
 * This prevents lazy loading and ensures immediate display
 */
function convertLazyImages(html: string): string {
  // Match img tags with lazy loading attributes
  // Pattern: <img ... src="placeholder" data-original="real-image" ...>
  const lazyImgRegex =
    /<img([^>]*?)\s+(data-original|data-src)\s*=\s*(['"]?)([^"'\s>]+)\3([^>]*?)>/gi;

  return html.replace(lazyImgRegex, (match, _beforeAttr, lazyAttr, quote, lazySrc, _afterAttr) => {
    // Extract the current src attribute if it exists
    const srcMatch = match.match(/\ssrc\s*=\s*(['"]?)([^"'\s>]+)\1/i);

    if (srcMatch) {
      // Image already has src, replace it with the lazy src
      const newSrc = `src=${quote}${lazySrc}${quote}`;

      // Replace the src attribute with the lazy-loaded URL
      let newMatch = match.replace(/\ssrc\s*=\s*(['"]?)([^"'\s>]+)\1/i, ' ' + newSrc);

      // Remove lazy loading class if present
      newMatch = newMatch.replace(
        /\sclass\s*=\s*(['"]?)([^"'\s>]*\blazy\b[^"'\s>]*)\1/i,
        (_classMatch, classQuote, classValue) => {
          const newClassValue = classValue.replace(/\blazy\b/g, '').trim();
          if (newClassValue) {
            return ` class=${classQuote}${newClassValue}${classQuote}`;
          }
          return '';
        }
      );

      // Remove the data-original/data-src attribute since we've moved it to src
      // Build a simple regex that matches the attribute
      // If quote is empty, match unquoted value; otherwise match quoted value
      let removeRegex: RegExp;
      if (quote) {
        // Match quoted value: data-original="value" or data-original='value'
        const quoteChar = quote === '"' ? '"' : "'";
        removeRegex = new RegExp(`${lazyAttr}=${quoteChar}[^${quoteChar}]+${quoteChar}`, 'gi');
      } else {
        // Match unquoted value: data-original=value
        removeRegex = new RegExp(`${lazyAttr}=[^\\s>]+`, 'gi');
      }
      newMatch = newMatch.replace(removeRegex, '');

      return newMatch;
    }

    // No src attribute, add one with the lazy src (this shouldn't happen with valid HTML)
    return match;
  });
}

/**
 * Proxy a specific attribute on a given HTML element tag
 * @param html HTML content
 * @param tagName HTML tag name to match (e.g., 'img', 'video', 'source')
 * @param attrName Attribute name to proxy (e.g., 'src', 'poster')
 * @param referer Optional referer URL
 * @param addReferrerPolicy Whether to add referrerpolicy="no-referrer" (default false, used for img)
 * @returns HTML with proxied attribute
 */
function proxyElementAttribute(
  html: string,
  tagName: string,
  attrName: string,
  referer?: string,
  addReferrerPolicy = false
): string {
  // Enhanced regex to handle element attributes with better pattern matching
  // Handles double quotes, single quotes, and unquoted values
  // Note: Unquoted values cannot contain spaces per HTML specification
  const elemRegex = new RegExp(
    `<${tagName}([^>]+)${attrName}\\s*=\\s*(['"]?)([^"'\\s>]+)\\2`,
    'gi'
  );

  return html.replace(elemRegex, (match, _attrs, quote, src) => {
    // CRITICAL FIX: Decode HTML entities before processing the URL
    // HTML attributes contain &amp; which should be decoded to & before URL encoding
    // For example: &amp; becomes &, then gets properly URL-encoded as %26
    const decodedSrc = decodeHTMLEntities(src);
    const proxiedUrl = getProxiedMediaUrl(decodedSrc, referer);

    // If proxying failed or returned the same URL, keep original
    if (!proxiedUrl || proxiedUrl === decodedSrc) {
      return match;
    }

    // Replace the attribute, preserving the original quote style
    const newAttr = `${attrName}=${quote}${proxiedUrl}${quote}`;
    const attrRegex = new RegExp(
      `${attrName}\\s*=\\s*${quote}${src.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}${quote}`,
      'i'
    );

    let result = match.replace(attrRegex, newAttr);

    // For img tags: Add referrerpolicy="no-referrer" to prevent browser from sending
    // Referer headers that might cause the proxy request to fail.
    if (addReferrerPolicy && !result.toLowerCase().includes('referrerpolicy')) {
      result = result + ' referrerpolicy="no-referrer"';
    }

    return result;
  });
}

/**
 * Decode HTML entities in a string
 * Handles common entities like &amp;, &lt;, &gt;, &quot;, &#39;, etc.
 */
function decodeHTMLEntities(text: string): string {
  const textarea = document.createElement('textarea');
  textarea.innerHTML = text;
  return textarea.value;
}

function unwrapJintiankanshaUrl(url: string): string {
  try {
    const parsed = new URL(url);
    if (parsed.hostname.toLowerCase() !== 'img2.jintiankansha.me') {
      return url;
    }
    const rawSrc = parsed.searchParams.get('src');
    if (!rawSrc) {
      return url;
    }
    const decoded = decodeURIComponent(rawSrc);
    if (decoded.startsWith('http://') || decoded.startsWith('https://')) {
      return decoded;
    }
    if (decoded.startsWith('/api/media/proxy')) {
      return decoded;
    }
    return url;
  } catch {
    return url;
  }
}

// unwrapJintiankanshaInHtml replaces img2.jintiankansha.me wrapper URLs in HTML.
// This avoids requests to img2 (which often return 403) and exposes the real image URL.
function unwrapJintiankanshaInHtml(html: string): string {
  const wrapperRegex = /https?:\/\/img2\.jintiankansha\.me\/get\?src=([^"'\s>]+)/gi;
  return html.replace(wrapperRegex, (match, rawValue) => {
    if (!rawValue) return match;
    try {
      const decoded = decodeURIComponent(rawValue);
      if (decoded.startsWith('/api/media/proxy')) {
        return decoded;
      }
      if (decoded.startsWith('http://') || decoded.startsWith('https://')) {
        return decoded;
      }
    } catch {
      return match;
    }
    return match;
  });
}
