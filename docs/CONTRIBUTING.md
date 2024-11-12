# Contribution Guidelines

### Pull Request guidelines
1. Follow the PR Templates
   - Use PR Template instructions, they allow to ensure consistency and clarity.
1. Avoid Self-Overrides
   - If automated tests fail (e.g., Prow tests), ask reviewers to override tests if necessary.
     - Reviewers may override tests at any time but should provide a reason and links to a job log that supports the override.
1. Communicate Clearly
   - Communication can be challenging
   - Explain your comments thoroughly
1. Provide Constructive Feedback
   - The best reviews include suggestions, requests for clarifications
   - Post the testing procedures and testing results directly to the PR for transparency..

### Issue guidelines
1. OADP Released Product Issues
   - The issues and tasks are best written up in Jira not GitHub.
1. OpenShift / MigTools org Issues
   - Use this for upcoming x.y releases within the x.y.z versioning schema.
   - For issues related to unreleased features and tasks, GitHub is the preferred platform.

### Focus
Our time and resources are limited, so we prioritize efforts that align with our project goals and deliver the most value to our customers. Issues and PRs will be tagged based on their alignment with project objectives:
- `priority/awaiting-more-evidence`: Needs additional information
- `priority/backlog`: Future consideration
- `priority/important-soon`: Near-term priority
- `priority/important-longterm`: Strategic importance

The team will triage these issues every 3 to 6 months. Please note that issues and PRs with these labels may not receive immediate review or comments.