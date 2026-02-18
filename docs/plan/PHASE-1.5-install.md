 Phase 1.5: Validation & Hardening

  1. Install Go, compile the project, fix any build errors
  2. Run make test-unit -- fix any failing unit tests
  3. Run aibox doctor -- see what passes/fails on this machine
  4. Run aibox start --workspace /tmp/test with Docker -- verify a container actually launches
  5. Run aibox shell -- verify we can exec into it
  6. Verify security controls inside the container:
    - capsh --print (zero capabilities?)
    - Write to / (should fail)
    - Write to /workspace (should succeed)
    - cat /proc/1/attr/current (AppArmor loaded?)
  7. Run make test-integration and make test-security** -- fix failures
  8. Fix all bugs found during validation