name: 🚀 Feature Request
description: Suggest an idea or enhancement for Hypasis Sync Protocol
title: '[FEATURE] '
labels: ['enhancement']
assignees: ''

body:
  - type: markdown
    attributes:
      value: |
        Thank you for suggesting a feature! Please provide details about your idea below.

  - type: textarea
    id: problem
    attributes:
      label: Is your feature request related to a problem?
      description: A clear description of what the problem or limitation is.
      placeholder: I am always frustrated when...
    validations:
      required: true

  - type: textarea
    id: solution
    attributes:
      label: Proposed Solution
      description: Describe the solution or feature you'd like to see implemented.
      placeholder: I propose that we add...
    validations:
      required: true

  - type: textarea
    id: alternatives
    attributes:
      label: Alternatives Considered
      description: Any alternative solutions or features you've considered.
      placeholder: An alternative approach would be...

  - type: textarea
    id: context
    attributes:
      label: Additional Context
      description: Add any other context, screenshots, or diagrams about the feature request.
