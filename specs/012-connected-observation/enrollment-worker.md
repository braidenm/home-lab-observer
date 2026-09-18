# Confined connected enrollment worker

Status: unused Linux composition; installed enrollment acceptance remains open.

The connected enrollment executable accepts only four compiled modes over
parent-owned anonymous pipes. Inputs are canonical JSON of at most 512 bytes;
there is no caller-selected endpoint, file path, shell, or retry policy.

- `enroll` consumes a one-use grant, initializes only the fixed enrollment
  state directory, performs one fixed-origin exchange, and returns the bound
  server/connector identity.
- `validate-enrollment` reads the pristine ready state and returns the private
  credential record for installer promotion.
- `validate-ledger` accepts only a pristine promoted ledger and returns a
  closed validation result.
- `validate-existing-ledger` reads a used ledger without changing it and
  returns the private binding-checked fingerprint protocol.

Every mode first verifies the confined non-root numeric principal. Failures
return one closed code without printing the grant, credential, fingerprint,
transport error, or ledger contents. The installer must separately prove the
loaded offline systemd policy, empty inherited credentials, exact process
ownership, and sealed handoff before writing input. Successful library tests
do not establish real enrollment, TLS, credential promotion, or installed VM
acceptance; none of those are authorized by this slice.
