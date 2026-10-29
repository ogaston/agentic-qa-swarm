# Demo launcher code generation plan

Context: Existing demo scripts orchestrate the warm environment, target commit, human confirmation and run monitoring. This launcher adds no service or database interface; it delegates to those scripts. Existing webhook configuration remains a prerequisite.

- [x] 1. Implement `scripts/demo/demo.sh`: interactive setup and dashboard startup; display dashboard URL, webhook URL and locally stored demo credentials; accept an existing SHA or prepare a healthy/oversell commit from a target clone; run and monitor with human confirmation. Stop on failures and preserve script output.
- [x] 2. Document usage and prerequisites in `scripts/demo/README.md`, including the credential display and push effects.
- [x] 3. Verify Bash syntax and exercise launcher branches with isolated mocked commands, without changing the cluster or pushing commits; record results and update state.
