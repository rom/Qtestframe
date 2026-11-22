// Who is offered the administrator guide (docs/admin-guide, /admin-guide).
import { can, P } from '@/lib/permissions'
import type { Me } from '@/lib/types'

/**
 * Whether the administrator guide is shown: to super administrators and to
 * organisation admins (tenant.manage). Its pages are bundled with the app
 * and published in the repository, so this keeps the help focused; it does
 * not protect the content.
 */
export function canSeeAdminGuide(me: Me | null | undefined): boolean {
  return can(me, P.tenantManage)
}
