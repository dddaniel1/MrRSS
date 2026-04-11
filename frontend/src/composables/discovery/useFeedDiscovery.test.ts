import { describe, expect, it } from 'vitest';
import en from '@/i18n/locales/en';
import { mapRecommendationErrorCode } from './useFeedDiscovery';

describe('useFeedDiscovery recommendation errors', () => {
  const t = (key: string) => {
    const parts = key.split('.');
    let current: any = en;
    for (const part of parts) {
      current = current?.[part];
    }
    return current ?? key;
  };

  it('maps known recommendation errors to localized messages', () => {
    expect(mapRecommendationErrorCode(t, 'recommendation_ai_not_configured')).toBe(
      'AI recommendation is not configured yet',
    );
    expect(mapRecommendationErrorCode(t, 'recommendation_validation_timed_out')).toBe(
      'Timed out while validating recommended websites',
    );
  });

  it('falls back to generic discovery failure for unknown error codes', () => {
    expect(mapRecommendationErrorCode(t, 'unknown_error')).toBe('Discovery failed: unknown_error');
  });
});
