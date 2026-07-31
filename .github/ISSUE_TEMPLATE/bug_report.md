name: 🐛 Bug Report
description: Create a report to help us improve Hypasis Sync Protocol
title: '[BUG] '
labels: ['bug', 'triage']
assignees: ''

body:
  - type: markdown
    attributes:
      value: |
        Thank you for reporting a bug! Please fill out the sections below to help us reproduce and fix it.

  - type: textarea
    id: description
    attributes:
      label: Bug Description
      description: A clear and concise description of what the bug is.
      placeholder: Describe what happened...
    validations:
      required: true

  - type: textarea
    id: reproduce
    attributes:
      label: Steps to Reproduce
      description: Steps to reproduce the behavior.
      placeholder: |
        1. Run './hypasis-sync --config=...'
        2. Execute query '...'
        3. See error
    validations:
      required: true

  - type: textarea
    id: logs
    attributes:
      label: Relevant Logs & Output
      description: Paste relevant terminal logs or error tracebacks here.
      render: shell

  - type: input
    id: environment
    attributes:
      label: Environment
      description: OS version, Go version, Go arch (e.g. Ubuntu 22.04 LTS, Go 1.21, x86_64)
      placeholder: Ubuntu 22.04 / Go 1.21.5
    validations:
      required: true
