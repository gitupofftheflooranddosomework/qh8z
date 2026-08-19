# qh8z final launch verification

Issue #3 is not complete merely because code has merged. This runbook defines the evidence required for the final Gate 6 checks on the actual launch environment.

Record the launch-candidate Git SHA at the top of the launch report and do not change code during the test without restarting the checklist on the new SHA.

## External prerequisites

Before the end-to-end test:

- `qh8z.com` and `www.qh8z.com` resolve to the intended production deployment;
- HTTPS has a valid publicly-trusted certificate and HTTP redirects to HTTPS;
- `support@qh8z.com`, `abuse@qh8z.com`, `privacy@qh8z.com`, and `security@qh8z.com` accept external mail and are monitored;
- the SMTP provider can send qh8z verification mail;
- production Web Risk credentials are active;
- Stripe has a recurring Pro price of exactly C$12.00/month and `STRIPE_PRO_PRICE_ID` references it;
- the Stripe production webhook points to `/api/v1/billing/webhook` and has the matching signing secret;
- offsite backup object storage is active;
- Prometheus/Alertmanager are healthy and the alert receiver has been test-fired; and
- at least one administrator has access to the abuse-review credential and server recovery procedures.

## End-to-end customer journey

Use a new email address/workspace that has never existed in production.

1. Register a new qh8z account.
2. Confirm the verification message is actually delivered through production SMTP.
3. Follow the verification link and confirm the account becomes verified.
4. Confirm the workspace starts on Free and displays the published Free limits.
5. Create a short link to a harmless public HTTPS destination.
6. Open the short link from a separate browser/client and confirm HTTP redirect behavior.
7. Return to the dashboard and confirm the visit appears in analytics.
8. Generate and scan/download the QR code and confirm it represents the short URL.
9. Edit the destination, confirm the link now redirects to the new destination, then disable/re-enable it and confirm redirect behavior changes accordingly.
10. Attempt to add a custom domain on Free and confirm the server refuses with the paid-plan entitlement response.
11. Start Pro checkout. Before paying, verify Stripe displays **C$12.00/month** and the correct qh8z product.
12. Complete the real production checkout using an approved low-risk launch transaction.
13. Confirm the Stripe webhook changes the workspace to Pro without manual database changes.
14. Confirm the dashboard shows Pro limits and allows a custom domain.
15. Add a launch-test custom domain, publish its qh8z TXT verification record plus routing DNS, and verify it through qh8z.
16. Confirm Caddy obtains a valid certificate for the verified custom hostname and the branded short link redirects successfully.
17. Open the Stripe customer portal from qh8z and confirm the correct subscription/customer is shown.
18. Cancel the launch-test subscription through the portal.
19. Confirm Stripe webhook processing changes qh8z state and the workspace falls back to Free when cancellation becomes effective.
20. Confirm custom-domain traffic is no longer authorized after paid entitlement ends, including with an already-issued certificate.

Record the short code, workspace ID, Stripe customer/subscription IDs, custom-domain ID, timestamps, and outcome without recording passwords, session cookies, API secrets, or payment-card data.

## Abuse workflow test

1. Create a harmless launch-test short link specifically for abuse testing.
2. Submit a public abuse report for that slug.
3. Confirm the internal reviewer sees the report.
4. Suspend the link and confirm the public redirect stops.
5. Review/resolve the report with a test note.
6. Unsuspend or delete the test link.
7. Confirm the relevant audit actions exist.

## Privacy and support channels

Send one external test message to each role mailbox and confirm receipt:

- support
- abuse
- privacy
- security

Submit one privacy access/deletion test request for the launch-test account and walk through the identity-verification process without deleting required operational evidence prematurely.

## Security negative tests

On the exact launch candidate:

- cross-workspace link/domain/analytics/billing access is denied;
- an API key without `links:write` cannot create/edit links;
- an API key without `analytics:read` cannot read analytics;
- an unverified account cannot create links;
- invalid Stripe webhook signatures are rejected;
- private/local/reserved destinations are rejected;
- known harmless public URLs pass reputation checking;
- a test block rule rejects its target;
- `/metrics`, `/readyz`, and `/internal/*` are not reachable through the public Caddy endpoint;
- the primary hostname cannot serve a custom-domain-only branded link; and
- a canceled Pro workspace cannot use custom-domain traffic.

## Backup and recovery evidence

Before declaring launch:

1. confirm a recent encrypted offsite backup exists;
2. restore a snapshot into a disposable database using the documented restore command;
3. confirm at least one known link/workspace exists in the restored database;
4. record the snapshot identifier and restore result; and
5. delete the disposable restore database after verification.

## Performance/failure evidence

Run the documented load test on a production-sized staging instance or a controlled production maintenance window. Record request count, error rate, RPS, p50, p95, and p99.

Launch threshold currently requires:

- error rate <= 1%;
- redirect p95 <= 500 ms under the documented 50-concurrency/60-second test; and
- no sustained qh8z 5xx/storage alert during the run.

Run the controlled PostgreSQL failure test and confirm readiness fails and then recovers.

## Public qh8z.com smoke test

From a network outside the production host:

```text
http://qh8z.com              -> redirects to HTTPS
https://qh8z.com/healthz     -> 200
https://qh8z.com/metrics     -> not publicly exposed
https://qh8z.com/readyz      -> not publicly exposed
https://qh8z.com/internal/... -> not publicly exposed
```

Also verify the home page, dashboard authentication behavior, Terms, Privacy Policy, Acceptable Use Policy, and abuse-report instructions are publicly accessible over HTTPS.

## Launch sign-off

Only after every step relevant to the production launch passes should the final Issue #3 boxes be checked and Issue #3 be closed. The launch report should include:

- launch Git SHA;
- production host/deployment identifier;
- CI run URL;
- backup/restore evidence;
- load/failure test results;
- security-review blocker closure;
- Stripe end-to-end result;
- role-mailbox tests; and
- final qh8z.com smoke-test timestamp.