# User Story Best Practices

## Overview

This guide provides comprehensive standards for creating high-quality user stories that follow INVEST principles and include testable GHERKIN acceptance criteria. User stories are the foundation of agile development, capturing user needs in a clear, actionable format.

## INVEST Principles

Every user story MUST satisfy all INVEST criteria:

### Independent
- Can be developed and delivered in any order
- Minimizes dependencies on other stories
- Can be tested and deployed independently
- Use epicId to group related work without creating dependencies

### Negotiable
- Implementation details remain flexible
- Focus on outcomes, not solutions
- Leaves room for technical creativity
- "What" not "how"

### Valuable
- Delivers clear value to users or business
- Answers "Why does this matter?"
- Ties to business outcomes or user problems
- Can articulate value in one sentence

### Estimable
- Team can estimate effort with reasonable confidence
- Requirements are clear enough to size
- Technical approach is understood at high level
- Acceptance criteria provide sufficient detail

### Small
- Fits within one sprint (1-8 story points)
- Typically completable in 1-5 days
- Break down larger stories into smaller pieces

### Testable
- Clear acceptance criteria exist
- Success is objectively verifiable
- Can write test cases before development
- Minimum 3 GHERKIN scenarios required

## User Story Template

```yaml
- id: US-XXX  # Auto-incremented by analyst
  epicId: EP-XXX  # Optional: link to parent epic
  title: As a [role] I want to [action] so that [benefit]
  description: |
    As a [user role]
    I want to [capability]
    So that [business value]

    [Additional context, business rules, constraints]

  priority: [high|medium|low]
  estimate: [1-8 story points]
  status: [draft|ready|in_progress|in_review|done|cancelled]
  owner: [team-name]

  acceptance:
    feature: "[Feature name]"
    scenarios:
      - name: Happy path
        given: "[Initial context]"
        when: "[User action]"
        then: "[Expected outcome]"

      - name: Validation error
        given: "[Invalid state]"
        when: "[User action]"
        then: "[Error handling]"

      - name: Edge case
        given: "[Boundary condition]"
        when: "[User action]"
        then: "[Graceful degradation]"

  nfr:  # Optional: non-functional requirements
    - key: [metric_name]
      target: "[measurable threshold]"

  links: []
  createdAt: YYYY-MM-DD
  updatedAt: YYYY-MM-DD
```

**Note**: The actual story files use Markdown format (see `story-template.md` in templates). This YAML structure shows the conceptual model for understanding story components.

## GHERKIN Acceptance Criteria

### Minimum Requirements
Every user story MUST have at least 3 scenarios:
1. **Happy path** - Normal successful flow
2. **Validation error** - Invalid input or business rule violation
3. **Edge case** - Boundary conditions, timeouts, unusual states

### GHERKIN Structure

**Given** - Initial context/preconditions
- Describe the state before action
- Set up necessary preconditions
- Examples: "A logged-in user", "An empty cart", "The API is responding"

**When** - Action or event
- Describe what happens
- Single, clear action
- Examples: "I submit the form", "The payment times out", "I click delete"

**Then** - Expected outcome
- Observable, testable result
- What changes in the system
- Examples: "I see a success message", "The order is created", "I receive an error"

### Scenario Format

```yaml
scenarios:
  - name: Descriptive scenario name
    given: "Initial state and preconditions"
    when: "Action taken by user or system"
    then: "Observable outcome that can be verified"
```

### Common Scenario Patterns

**Authentication:**
```yaml
- name: Successful login
  given: "Valid user credentials"
  when: "I submit the login form"
  then: "I am redirected to dashboard and see welcome message"

- name: Invalid credentials
  given: "Incorrect password"
  when: "I submit the login form"
  then: "I see 'Invalid credentials' error and remain on login page"

- name: Account locked
  given: "Account locked after 5 failed attempts"
  when: "I try to log in with correct credentials"
  then: "I see 'Account locked' message and cannot access system"
```

**Form Validation:**
```yaml
- name: Valid form submission
  given: "All required fields filled with valid data"
  when: "I submit the form"
  then: "Data is saved and I see success confirmation"

- name: Missing required field
  given: "Email field is empty"
  when: "I submit the form"
  then: "I see 'Email is required' error and form is not submitted"

- name: Invalid format
  given: "Email field contains 'notanemail'"
  when: "I submit the form"
  then: "I see 'Invalid email format' error below the field"
```

**API Operations:**
```yaml
- name: Successful API call
  given: "Valid authentication token and request payload"
  when: "I send POST request to /api/resource"
  then: "I receive 201 Created with resource ID in response"

- name: Unauthorized request
  given: "No authentication token"
  when: "I send POST request to /api/resource"
  then: "I receive 401 Unauthorized response"

- name: API timeout
  given: "The API does not respond within 3 seconds"
  when: "I send POST request to /api/resource"
  then: "I receive timeout error and request is retried per policy"
```

### Writing Guidelines

**Be Specific:**
❌ "The system works correctly"
✅ "The order status changes to 'confirmed' and confirmation email is sent"

**Use Business Language:**
❌ "The database row is updated with status='active'"
✅ "The user account becomes active and can log in"

**Focus on Outcomes:**
❌ "The React component re-renders"
✅ "The product list displays with updated prices"

**One Action Per Scenario:**
❌ "When I fill form, submit, and confirm"
✅ "When I submit the completed form" (previous steps in given)

## Non-Functional Requirements (NFR)

Specify measurable NFRs when relevant to the story:

**Performance:**
```yaml
- key: response_time_p95_ms
  target: "<= 500"
```

**Reliability:**
```yaml
- key: availability
  target: ">= 99.9%"
```

**Security:**
```yaml
- key: encryption
  target: "TLS 1.3 in transit, AES-256 at rest"
```

**Scalability:**
```yaml
- key: max_concurrent_users
  target: ">= 10000"
```


## Epic Integration

Stories can link to epics for strategic alignment:

```yaml
epics:
  - id: EP-001
    title: Streamlined Checkout
    outcome: Reduce cart abandonment by 20%
    kpi:
      - name: checkout_completion_rate
        target: ">= 80%"

stories:
  - id: US-001
    epicId: EP-001  # Links to epic above
    title: As a customer I want to save payment methods...
```

## Quality Checklist

**Story Completeness:**
- [ ] Title follows "As a...I want...so that" format
- [ ] Clear business value articulated
- [ ] All INVEST criteria satisfied
- [ ] Minimum 3 GHERKIN scenarios
- [ ] Priority and estimate provided
- [ ] Owner assigned

**Acceptance Criteria:**
- [ ] Scenarios have descriptive names
- [ ] Given/When/Then are clear and testable
- [ ] Covers success and failure cases
- [ ] No implementation details
- [ ] Uses business language

## Anti-Patterns to Avoid

❌ **Too Large**: Story >8 points (break it down)
❌ **Technical Jargon**: "Update Redux store" → "Save user preferences"
❌ **Vague Outcomes**: "Works correctly" → "Order confirmed and email sent"
❌ **Implementation Details**: "Use React hooks" → Focus on user outcome
❌ **Missing Error Cases**: Only happy path → Include validation errors
❌ **Dependent Stories**: Stories block each other → Make independent

## Complete Example

```yaml
- id: US-042
  epicId: EP-005
  title: As a customer I want to track my order so that I know when it will arrive
  description: |
    As a customer
    I want to track my order status and location
    So that I can plan to receive my delivery

    Business context: Reduces support calls for order status inquiries.
    Integrates with 3rd party logistics API.

  priority: high
  estimate: 5
  status: ready
  owner: team-commerce

  acceptance:
    feature: "Order tracking"
    scenarios:
      - name: View shipped order status
        given: "Order is shipped with tracking number available"
        when: "I view my order details page"
        then: "I see current status, estimated delivery, and tracking map"

      - name: Order not yet shipped
        given: "Order is confirmed but not shipped"
        when: "I view order details"
        then: "I see 'Processing' status and estimated ship date"

      - name: Tracking service timeout
        given: "Logistics API does not respond within 3 seconds"
        when: "I view order details"
        then: "I see last known status with 'Live tracking temporarily unavailable' message"

  nfr:
    - key: tracking_refresh_seconds
      target: "30"
    - key: api_timeout_ms
      target: "3000"

  links:
    - https://docs.shiptrack.example/api

  createdAt: 2025-11-26
  updatedAt: 2025-11-26
```

This example demonstrates:
- Clear INVEST principles applied
- 3 diverse GHERKIN scenarios (happy/error/edge)
- Relevant NFRs for API integration
- Proper linking and metadata
