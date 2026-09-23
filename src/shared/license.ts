import type { FeatureId } from './features'
import type { LicenseAuthStatus, LicenseEntitlement } from './types'

export function featureEntitlement(featureId: FeatureId): LicenseEntitlement {
  switch (featureId) {
    case 'green-window':
    case 'virtual-camera':
    case 'impact-gift':
    case 'gift-screen':
      return 'overlay'
    default:
      return 'slot'
  }
}

export function hasEntitlement(status: LicenseAuthStatus, entitlement: LicenseEntitlement): boolean {
  return status.loggedIn && (status.mode === 'local' || status.features.includes(entitlement))
}
