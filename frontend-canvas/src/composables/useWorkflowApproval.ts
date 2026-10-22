import { computed, ref, type Ref } from 'vue'

/** Confirmation belongs to the current workflow and its current references. */
export function useWorkflowApproval(fingerprint: Ref<string>) {
  const approvalRequired = ref(false)
  const approvedFingerprint = ref('')
  const keyframeApproved = computed({
    get: () => approvedFingerprint.value === fingerprint.value,
    set: (checked: boolean) => { approvedFingerprint.value = checked ? fingerprint.value : '' },
  })
  function loadApproval(data: Record<string, any>) {
    approvalRequired.value = data.approvalRequired === true
    approvedFingerprint.value = data.approvedFingerprint || ''
  }
  function approvalState() {
    return { approvalRequired: approvalRequired.value, approvedFingerprint: approvedFingerprint.value }
  }
  return { approvalRequired, keyframeApproved, loadApproval, approvalState }
}
