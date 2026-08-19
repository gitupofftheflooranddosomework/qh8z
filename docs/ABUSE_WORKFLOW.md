# qh8z abuse and safety workflow

This workflow defines how qh8z receives, triages, enforces, documents, and closes abuse reports. It applies to the public abuse API, `abuse@qh8z.com`, and reports received through support or security channels.

## Public reporting

A reporter should provide:

- the complete qh8z short link;
- category: malware, phishing, scam, spam, or other;
- a concise explanation and relevant evidence; and
- an email address only if a response is requested.

The public API intentionally returns a generic accepted response for unknown slugs so it cannot be used as a link-existence oracle.

## Triage targets

These are operational response targets, not guarantees:

| Severity | Examples | Initial triage target |
| --- | --- | --- |
| Critical | active malware, credential phishing, child exploitation, credible imminent threat | 1 hour |
| High | active fraud/scam, malicious download, repeated enforcement evasion | 4 hours |
| Normal | spam, policy complaints, trademark/copyright claims, non-urgent disputes | 1 business day |

Critical reports can be actioned immediately before contacting the account owner.

## Reviewer procedure

1. Open the abuse report in the internal review API/tooling.
2. Confirm the reported qh8z slug and current destination without executing untrusted files or submitting credentials.
3. Review qh8z safety signals: managed URL rules, reputation result, prior reports, audit history, link/workspace history, and repeat-abuse patterns.
4. Classify the report as substantiated, unsubstantiated, duplicate, or requiring more information.
5. For substantiated urgent abuse, suspend the link immediately and add a review note explaining the factual basis.
6. Add a domain/host block rule when the evidence supports blocking future links to the same malicious destination family.
7. Escalate repeat or severe workspace abuse for account/workspace restriction or termination.
8. Mark the report reviewed/resolved only after the enforcement decision and notes are recorded.
9. Reply to the reporter when appropriate without exposing private account data or internal security details.

## Appeals and false positives

Account owners can contact `abuse@qh8z.com` with the affected short link and relevant evidence. A reviewer who did not make the original decision should handle an appeal where staffing permits.

An allow rule should be used only when there is a documented false positive and the destination has been independently reviewed. Static protection against private/local/reserved destinations must never be bypassed by an allow rule.

## Legal and rights complaints

For copyright, trademark, privacy, or other rights complaints, request enough information to identify:

- the complaining party and contact method;
- the qh8z link at issue;
- the right allegedly affected;
- the factual/legal basis for the complaint; and
- any authorization when a representative submits the complaint.

Do not suspend merely because a complaint was received when the underlying dispute is unclear and there is no urgent safety issue. Preserve the complaint and enforcement record when reasonably necessary for dispute resolution or legal compliance.

## Law-enforcement requests

Requests for non-public user data must be referred to the operator responsible for legal process. Do not provide account data through the ordinary abuse mailbox merely because the requester claims to be law enforcement. Validate the request and disclose only information authorized or required by applicable law.

Emergency requests involving credible imminent danger should be escalated immediately.

## Privacy and evidence handling

Do not copy unnecessary passwords, payment-card data, private documents, or sensitive personal information into abuse notes. Store only information reasonably necessary for review and evidence. Do not click unknown executable downloads on an administrator workstation.

## Launch prerequisites

Before public launch:

- `abuse@qh8z.com` must accept external mail and be monitored;
- the public abuse API must be reachable;
- at least one operator must have the qh8z admin credential needed to review reports and suspend/unsuspend links;
- the critical-abuse notification path must be tested; and
- a test report must be submitted, reviewed, suspended, and resolved end-to-end.