import { useToast } from 'primevue/usetoast'
import { useConfirm } from 'primevue/useconfirm'
import { errorMessage } from '@/lib/api'

export function useNotify() {
  const toast = useToast()
  const confirm = useConfirm()
  return {
    ok(summary: string, detail?: string) {
      toast.add({ severity: 'success', summary, detail, life: 2500 })
    },
    info(summary: string, detail?: string) {
      toast.add({ severity: 'info', summary, detail, life: 3500 })
    },
    fail(e: unknown, summary = 'Request failed') {
      toast.add({ severity: 'error', summary, detail: errorMessage(e), life: 6000 })
    },
    confirmDanger(message: string, accept: () => void | Promise<void>, header = 'Please confirm') {
      confirm.require({
        message,
        header,
        icon: 'pi pi-exclamation-triangle',
        rejectProps: { label: 'Cancel', severity: 'secondary', outlined: true },
        acceptProps: { label: 'Delete', severity: 'danger' },
        accept,
      })
    },
  }
}
