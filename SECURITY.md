# Security Policy

## Scope

Yteam Platform is an open-source platform for authorized penetration testing,
bug bounty research, security QA, and defensive server engineering. It is not
intended for unauthorized access, mass scraping, spam, credential stuffing,
denial of service, or bypassing protections on systems without written
permission.

## Reporting a vulnerability in Yteam itself

Do not open a public issue for an unpatched security vulnerability. Send a
private report to the repository maintainers with:

- affected version/commit;
- operating system and runtime versions;
- minimal reproduction;
- impact and required permissions;
- a proposed fix if available.

Do not include live credentials, tokens, customer data, or unredacted logs.

## Safe defaults

The platform defaults to:

- exact-target scope validation;
- low-rate/read-first probes;
- no destructive actions or DoS;
- no credential stuffing;
- no customer-object access;
- no automatic cloud-resource claims;
- redacted evidence;
- hypothesis-before-finding classification;
- no automatic report submission.

These controls are part of the product's trust model and should not be
removed from public builds.
