# HarborClass Fix Check Report (Static-Only)

Date: 2026-04-18
Scope: Verification of previously listed Blocker/High/Medium issues in .tmp/audit_report-1.md

## 1. Verdict
- Overall conclusion for previously listed issues: Pass
- Re-evaluated result compared to prior report: All three previously open issues are now fixed with static code and test evidence.

## 2. Static Boundary
- Performed static review only.
- Did not run project, Docker, tests, or external services.
- Conclusions below are based on code, docs, and test artifacts only.

## 3. Issue-by-Issue Fix Verification

### Issue A (High)
Title: Admin-configured permission overlay not enforced on all critical routes
Prior status: Open
Current status: Fixed

Evidence:
- Audit search now uses policy overlay via h.can and ActionSearchAudit:
  - repo/internal/http/handlers/audit.go:19
- Audit export now uses policy overlay via h.can and ActionExportAudit:
  - repo/internal/http/handlers/audit.go:37
- Refund approval now uses policy overlay via h.can and ActionApproveRefund:
  - repo/internal/http/handlers/admin.go:100
- Overlay loading and policy plumbing in handler base:
  - repo/internal/http/handlers/handlers.go:49
  - repo/internal/http/handlers/handlers.go:55
- Integration test confirms overlay enforcement on next protected action:
  - repo/internal/http/admin_handler_integration_test.go:62

Conclusion:
- The previously identified hardcoded role checks on those critical routes were replaced with action-based policy checks.

### Issue B (High)
Title: Booking read endpoint had incomplete object-level authorization
Prior status: Open
Current status: Fixed

Evidence:
- Booking read now checks h.can(ActionManageOwnOrder) with owner + org + class target:
  - repo/internal/http/handlers/booking.go:88
- Relevant action semantics in auth policy:
  - repo/internal/auth/scope.go:66
- Security regression tests added for denied access:
  - Foreign student denied: repo/internal/http/booking_handler_integration_test.go:159
  - Cross-org admin denied: repo/internal/http/booking_handler_integration_test.go:183
  - Dispatcher denied: repo/internal/http/booking_handler_integration_test.go:207

Conclusion:
- Object-level authorization gap on booking read is closed with policy-based enforcement and targeted tests.

### Issue C (Medium)
Title: API spec mismatch for unsubscribe token contract
Prior status: Open
Current status: Fixed

Evidence:
- API spec now documents user, category, and token query params and 403 behavior for invalid signatures:
  - docs/api-spec.md:32
- Handler enforces token verification:
  - repo/internal/http/handlers/booking.go:238
- Security tests for unsigned and forged tokens:
  - repo/internal/http/booking_handler_integration_test.go:138
  - repo/internal/http/booking_handler_integration_test.go:148

Conclusion:
- Documentation and implementation are now aligned for the one-click unsubscribe security contract.

## 4. Summary of Re-Audit Outcome
- Fixed from prior report:
  - Permission overlay enforcement on critical routes: Fixed
  - Booking read object-level authorization: Fixed
  - Unsubscribe API spec/token contract mismatch: Fixed

- Remaining status for these specific items:
  - No open Blocker/High/Medium findings remain from the previously listed three issues.

## 5. Optional Follow-up (Non-blocking)
- Consider updating docs/api-spec.md audit endpoint descriptions that currently say Admin only, to explicitly mention default admin-only with optional admin-configured policy grants, matching runtime overlay behavior.
  - docs/api-spec.md:75
  - docs/api-spec.md:76
