const PRO_FIELDS = ['expiresAt', 'maxVisits'];

function currentPlanIsPro() {
  return document.getElementById('planBadge')?.textContent?.trim().toUpperCase() === 'PRO';
}

function syncProControls() {
  const pro = currentPlanIsPro();
  for (const formId of ['createLinkForm', 'editForm']) {
    const form = document.getElementById(formId);
    if (!form) continue;
    for (const name of PRO_FIELDS) {
      const input = form.elements.namedItem(name);
      if (!(input instanceof HTMLInputElement)) continue;
      input.disabled = !pro;
      input.title = pro ? '' : 'QH8Z Pro control. Existing saved values are preserved after a downgrade.';
      input.closest('label')?.classList.toggle('pro-control-locked', !pro);
    }
  }
}

document.addEventListener('DOMContentLoaded', () => {
  syncProControls();
  const badge = document.getElementById('planBadge');
  if (badge) new MutationObserver(syncProControls).observe(badge, { childList: true, characterData: true, subtree: true });
});

// The edit controller populates values after the click. Re-apply the lock on
// the next task so a former Pro user sees the saved controls but FormData omits
// them from ordinary Free edits, allowing the backend to preserve those values.
document.addEventListener('click', event => {
  if (event.target.closest('[data-action="edit"]')) setTimeout(syncProControls, 0);
}, true);
