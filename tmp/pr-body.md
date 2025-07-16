
# 🚨 Trivy Vulnerability Report (High/Critical)

| Target | Package | Severity | Title | CVE | Installed | Fixed |
|--------|---------|----------|-------|-----|-----------|-------|
| go.mod | golang.org/x/crypto | CRITICAL | golang.org/x/crypto/ssh: Misuse of ServerConfig.PublicKeyCallback may cause authorization bypass in golang.org/x/crypto | CVE-2024-45337 | v0.0.0-20220722155217-630584e8d5aa | 0.31.0 |
| go.mod | golang.org/x/crypto | HIGH | golang.org/x/crypto/ssh: Denial of Service in the Key Exchange of golang.org/x/crypto/ssh | CVE-2025-22869 | v0.0.0-20220722155217-630584e8d5aa | 0.35.0 |
=======
# 🚨 Gosec Vulnerability Report (High/Critical)
* File: /home/runner/work/pf9-oidc-proxy/pf9-oidc-proxy/test/e2e/framework/helper/deploy.go
    • Line: 502
    • Rule ID: G115
    • Details: integer overflow conversion int32 -> uint64
    • Confidence: MEDIUM
    • Severity: HIGH
* File: /home/runner/work/pf9-oidc-proxy/pf9-oidc-proxy/test/e2e/framework/helper/token.go
    • Line: 39-41
    • Rule ID: G402
    • Details: TLS MinVersion too low.
    • Confidence: HIGH
    • Severity: HIGH

