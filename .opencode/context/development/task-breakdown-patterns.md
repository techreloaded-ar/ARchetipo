# Task Breakdown Patterns

This document provides pattern-based guidance for breaking down user stories into executable technical tasks across any software project. Use these patterns to ensure consistent, comprehensive task planning, adapting technology names to your specific stack.

## Pattern Categories

- [Authentication & Authorization](#authentication--authorization-pattern)
- [CRUD Operations](#crud-operations-pattern)
- [Payment Integration](#payment-integration-pattern)
- [File Upload & Storage](#file-upload--storage-pattern)
- [Search & Filtering](#search--filtering-pattern)
- [Content Management](#content-management-pattern)
- [API Integration](#api-integration-pattern)
- [Refactoring](#refactoring-pattern)

---

## Authentication & Authorization Pattern

**Typical Story:** "As a user I want to register and login so that I can access protected features"

### Task Breakdown (adapt to your tech stack)

**1. Data Layer (3 tasks)**
- Create database migration for User model (email, passwordHash, role, createdAt, updatedAt) [Use project ORM: Prisma, Alembic, Flyway, etc.]
- Define User entity with unique email constraint and indexes [Adapt role enum to project domain]
- Generate ORM client and run migration on dev database [If ORM requires code generation]

**2. Business Logic (4 tasks)**
- Create authentication module with password hashing library [Use project backend framework and hashing lib: bcrypt, argon2, etc.]
- Implement AuthService.register() with email uniqueness check and password hashing
- Implement AuthService.login() with token generation (define expiry based on security requirements)
- Implement password reset service with token generation and expiry

**3. API Layer (4 tasks)**
- Create auth controller with POST /api/auth/register (validation: email format, password requirements) [Use project backend framework]
- Add POST /api/auth/login endpoint with rate limiting [Use rate limiting library from project stack]
- Add POST /api/auth/reset-password endpoint
- Create authentication guard for route protection and role-based access control [Use project auth pattern]

**4. UI Layer (3 tasks)**
- Build /register page with form validation and styling [Use project frontend framework and form library]
- Build /login page with error handling and redirect logic
- Build /forgot-password page with email validation

**5. Testing (4 tasks)**
- Add unit tests for User model validation (email format, unique constraint) [Use project test framework]
- Add unit tests for AuthService methods (register, login, token generation)
- Add integration tests for auth endpoints (201 success, 400/409 errors) [Use project integration test framework]
- Add e2e tests for registration and login flows [Use project E2E framework]

**6. Documentation (1 task)**
- Update API documentation with auth endpoints, request/response schemas [Use project API doc tool]

### Key Considerations (adapt to project context)
- Define default role for new user registrations based on project domain
- Token payload should include userId and role for authorization checks
- Password requirements: define based on security policy (e.g., min 8 chars, complexity rules)
- Rate limiting prevents brute force attacks on authentication endpoints
- Secure token storage: prefer HttpOnly cookies over localStorage (XSS protection)
- Password reset link should have expiration and be single-use
- Consider regulatory requirements: GDPR, user consent, audit logging

### Common Pitfalls
- Forgetting to hash password before saving to database
- Not handling duplicate email error (check ORM-specific error codes)
- Missing rate limiting on authentication endpoints (security vulnerability)
- Storing tokens in localStorage (XSS risk - use HttpOnly cookie instead)
- Not testing concurrent registration attempts
- Forgetting to validate role enum values or permissions

---

## CRUD Operations Pattern

**Typical Story:** "As a content creator I want to create and manage items so that users can access them"

### Task Breakdown (adapt to your tech stack)

**1. Data Layer (3 tasks)**
- Create database migration for [Entity] model (core fields, status enum if applicable, foreign keys, timestamps) [Use project ORM]
- Define [Entity] entity with relationships and indexes on frequently queried fields
- Generate ORM client and run migration [If ORM requires code generation]

**2. Business Logic (4 tasks)**
- Create [Entity] module/app [Use project backend framework structure]
- Implement [Entity]Service with CRUD methods (create, findAll, findOne, update, delete)
- Add business logic for state transitions (if entity has workflow states)
- Implement ownership/permission validation (users can only edit their own resources)

**3. API Layer (5 tasks)**
- Create [Entity] controller with POST /api/[entities] (create, with role check if needed) [Use project backend framework]
- Add GET /api/[entities] (list all, with pagination and filtering)
- Add GET /api/[entities]/:id (get single)
- Add PATCH/PUT /api/[entities]/:id (update, with ownership/permission check)
- Add DELETE /api/[entities]/:id (soft or hard delete based on requirements, with ownership check)

**4. UI Layer (4 tasks)**
- Build /[entities] page with list, pagination, and filters [Use project frontend framework]
- Build /[entities]/new page (create form with validation)
- Build /[entities]/:id page (detail view with conditional edit button)
- Build /[entities]/:id/edit page (edit form with validation)

**5. Testing (4 tasks)**
- Add unit tests for [Entity] model validation [Use project test framework]
- Add unit tests for [Entity]Service CRUD operations
- Add integration tests for [Entity] endpoints (CRUD operations, authorization checks) [Use project integration test framework]
- Add e2e tests for complete create/view/edit/delete flows [Use project E2E framework]

**6. Documentation (1 task)**
- Update API documentation with [Entity] endpoints and schemas [Use project API doc tool]

### Key Considerations (adapt to project context)
- If entity has workflow states, define clear state transition rules
- Determine visibility rules (who can see what states/items)
- Choose soft delete vs hard delete based on data retention requirements
- Store monetary values as integers (cents) to avoid floating-point precision issues
- Always implement pagination for list endpoints (define reasonable default, e.g., 20-50 items)
- Implement filtering on commonly searched fields

### Common Pitfalls
- Not implementing pagination (performance issue with large datasets)
- Forgetting ownership/permission checks (security vulnerability)
- Hard delete when soft delete is needed (breaks data integrity/history)
- Not validating state transitions (invalid workflows)
- Missing indexes on frequently queried fields (performance degradation)

---

## Payment Integration Pattern

**Typical Story:** "As a user I want to purchase premium content so that I can access exclusive features"

### Task Breakdown (adapt to your tech stack)

**1. Data Layer (3 tasks)**
- Create database migration for Purchase model (userId FK, itemId FK, amount, currency, paymentProvider enum, paymentTransactionId, status enum, timestamps) [Use project ORM]
- Define Purchase entity with relationships and unique constraint on (userId, itemId) if one-time purchase
- Generate ORM client and run migration [If ORM requires code generation]

**2. Business Logic (5 tasks)**
- Create payments module with payment provider SDK integration [Use project backend framework and payment provider: Stripe, PayPal, Square, etc.]
- Implement PaymentsService.createPayment() for payment creation
- Implement PaymentsService.handleWebhook() for payment status updates
- Create PurchasesService to track purchases and unlock content
- Add logic for revenue sharing or commission calculation (if applicable to domain)

**3. API Layer (4 tasks)**
- Create payments controller with POST /api/payments/intent (initiate payment) [Use project backend framework]
- Add POST /api/webhooks/[provider] (handle payment provider webhooks)
- Create purchases controller with GET /api/purchases (user's purchase history)
- Add GET /api/purchases/[itemId]/access (check if user has access to purchased item)

**4. UI Layer (4 tasks)**
- Build /[items]/:id/purchase page with payment provider integration [Use project frontend framework and payment provider UI components]
- Add payment confirmation UI with success/error states
- Build /my-purchases page showing user's purchase history
- Add conditional access/download button on item detail (only if purchased)

**5. Integration Layer (3 tasks)**
- Configure payment provider webhook endpoint with signature verification
- Implement idempotency handling for webhook retry scenarios
- Add webhook event logging and error monitoring

**6. Testing (5 tasks)**
- Add unit tests for payment creation logic [Use project test framework]
- Add unit tests for webhook handler (payment status transitions)
- Add integration tests for payment endpoints with provider test mode [Use project integration test framework]
- Add e2e tests for complete purchase flow (payment → webhook → content unlock) [Use project E2E framework]
- Add tests for idempotency and duplicate webhook handling

**7. Documentation (1 task)**
- Update API documentation with payment endpoints and webhook format [Use project API doc tool]

### Key Considerations (adapt to project context)
- Use payment provider's secure payment flow (e.g., PaymentIntents for Stripe, PayPal Checkout)
- Always verify webhook signatures to prevent spoofing
- Implement idempotency to prevent duplicate charges on retries
- Store monetary amounts as integers (cents/smallest unit) to avoid precision errors
- Consider unique constraints on purchases based on business rules
- Define revenue sharing or commission logic based on business model
- Handle webhook retries gracefully (providers typically retry for several days)

### Common Pitfalls
- Not verifying webhook signatures (critical security vulnerability)
- Missing idempotency handling (can result in duplicate charges)
- Storing prices as floats (precision errors, especially for currency)
- Not handling payment failures or cancellations gracefully
- Forgetting to test webhook retry and failure scenarios
- Not logging payment events (makes debugging very difficult)

---

## File Upload & Storage Pattern

**Typical Story:** "As a user I want to upload files so that I can share content or attach documents"

### Task Breakdown (adapt to your tech stack)

**1. Data Layer (3 tasks)**
- Create database migration for Media/File model (type/category, fileName, storageKey, storageBucket, fileSize, mimeType, relatedEntityId FK, uploadedBy FK, timestamps) [Use project ORM]
- Define Media entity with relationships to parent entity and user
- Generate ORM client and run migration [If ORM requires code generation]

**2. Storage Layer (4 tasks)**
- Configure cloud storage client [Use project storage provider: AWS S3, MinIO, Azure Blob, Google Cloud Storage]
- Implement StorageService.uploadFile() with chunked/multipart upload support
- Implement StorageService.generateSecureUrl() for temporary access (define expiry based on security requirements)
- Implement StorageService.deleteFile() for cleanup

**3. Business Logic (3 tasks)**
- Create media/file module [Use project backend framework]
- Implement MediaService with upload validation (file type whitelist, size limits based on file type)
- Implement MediaService.createFileRecord() to track uploads in database

**4. API Layer (4 tasks)**
- Create media controller with POST /api/media/upload (multipart/form-data, requires authentication) [Use project backend framework]
- Add validation for file type and size
- Add GET /api/media/:id/download (generates secure URL, with access control checks)
- Add DELETE /api/media/:id (delete with ownership check)

**5. UI Layer (3 tasks)**
- Build file upload component with drag-and-drop [Use project frontend framework and file upload library]
- Add upload progress indicator
- Build media gallery/list component for displaying uploaded files

**6. Testing (4 tasks)**
- Add unit tests for file validation logic [Use project test framework]
- Add unit tests for secure URL generation
- Add integration tests for upload endpoint (success, size limit, type validation) [Use project integration test framework]
- Add e2e tests for complete upload and download flow [Use project E2E framework]

**7. Documentation (1 task)**
- Update API documentation with media upload endpoints and supported file types [Use project API doc tool]

### Key Considerations (adapt to project context)
- Define file type whitelist based on business requirements
- Implement file content validation for structured files (e.g., XML, JSON validation)
- Consider file optimization (resize images, compress videos) before/after upload
- Set secure URL expiry based on security requirements
- Configure storage with private access (no public URLs unless required)
- Consider virus/malware scanning for user-uploaded files
- Define file size limits per file type and total storage per user

### Common Pitfalls
- Not validating file types (major security risk - arbitrary file upload)
- Missing file size limits (storage costs, DoS risk)
- Using public storage URLs (bypasses access control)
- Not cleaning up orphaned files when parent entity is deleted (storage waste)
- Forgetting to handle upload failures and partial uploads
- Not optimizing large media files (bandwidth costs and slow loading)

---

## Search & Filtering Pattern

**Typical Story:** "As a user I want to search and filter items so that I can find what matches my needs"

### Task Breakdown (adapt to your tech stack)

**1. Data Layer (2 tasks)**
- Add database indexes on filterable fields (identify commonly filtered attributes) [Use project ORM migration]
- Create full-text search index on searchable text fields (use database-specific FTS: PostgreSQL tsvector, MySQL FULLTEXT, MongoDB text index)

**2. Business Logic (3 tasks)**
- Extend [Entity]Service with search() method using ORM query builder [Use project ORM]
- Implement dynamic query builder for WHERE clauses based on active filters
- Add sorting logic (define sortable fields based on business requirements)

**3. API Layer (2 tasks)**
- Extend GET /api/[entities] with query params (q for search, filter params, sort, page, limit) [Use project backend framework]
- Add response metadata (totalCount, currentPage, totalPages, hasMore/hasNextPage)

**4. UI Layer (4 tasks)**
- Build search input component with debounced input [Use project frontend framework]
- Build filter component (dropdowns, checkboxes, range sliders based on filter types)
- Add sorting dropdown/controls
- Implement pagination controls

**5. Testing (3 tasks)**
- Add unit tests for query builder logic [Use project test framework]
- Add integration tests for search endpoint (full-text search, filters, sorting, pagination) [Use project integration test framework]
- Add e2e tests for complete search and filter flow [Use project E2E framework]

**6. Documentation (1 task)**
- Update API documentation with search query parameters and filter options [Use project API doc tool]

### Key Considerations (adapt to project context)
- Use database-appropriate full-text search (PostgreSQL tsvector, Elasticsearch, etc.)
- Debounce search input (typical: 300ms) to reduce API calls
- Always implement pagination for search results (define reasonable default)
- Define filter combination logic (AND vs OR) based on business requirements
- Define default sort order
- Consider caching frequent search queries (with appropriate TTL)

### Common Pitfalls
- Not debouncing search input (excessive API calls and poor UX)
- Missing indexes on filterable fields (slow queries as dataset grows)
- Not implementing pagination (performance issue and poor UX)
- Forgetting to validate and sanitize query parameters (security risk)
- Not handling empty results gracefully (poor UX)

---

## Content Management Pattern

**Typical Story:** "As a moderator I want to review and approve user-submitted content so that only quality content is published"

### Task Breakdown (adapt to your tech stack)

**1. Data Layer (2 tasks)**
- Add status transitions audit log (statusHistory or separate audit table) [Use project ORM]
- Create database migration for moderation/approval workflow states

**2. Business Logic (3 tasks)**
- Extend [Entity]Service with moderation methods (approve, reject, requestChanges) [Use project backend framework]
- Add status transition validation (define allowed transitions, e.g., PENDING → APPROVED/REJECTED by moderator role)
- Implement notifications for content creators on status changes (email, in-app, etc.)

**3. API Layer (3 tasks)**
- Add PATCH /api/admin/[entities]/:id/approve endpoint (moderator/admin role required) [Use project backend framework]
- Add PATCH /api/admin/[entities]/:id/reject endpoint with reason/feedback field
- Add GET /api/admin/[entities]/pending (list items awaiting review)

**4. UI Layer (3 tasks)**
- Build /admin/[entities] page with pending queue [Use project frontend framework]
- Build review modal/panel with approve/reject controls and reason textarea
- Add status badges/indicators to show moderation state

**5. Testing (3 tasks)**
- Add unit tests for status transition logic [Use project test framework]
- Add integration tests for admin endpoints (authorization checks) [Use project integration test framework]
- Add e2e tests for complete approval workflow [Use project E2E framework]

**6. Documentation (1 task)**
- Update API documentation with moderation endpoints [Use project API doc tool]

### Key Considerations (adapt to project context)
- Define which roles can moderate content (admin, moderator, etc.)
- Implement notifications to keep content creators informed
- Rejection reason/feedback should be required (helps creators improve)
- Log status history for audit trail and compliance
- Define which states are visible to which user roles
- Consider automated moderation rules before manual review (profanity filter, etc.)

### Common Pitfalls
- Not enforcing proper role authorization (security vulnerability)
- Missing notifications (poor UX and user frustration)
- Forgetting to log status changes (compliance and audit issues)
- Allowing invalid status transitions (broken workflows)
- Not providing actionable rejection feedback

---

## API Integration Pattern

**Typical Story:** "As a user I want to auto-populate data from external sources so that I can save time entering information"

### Task Breakdown (adapt to your tech stack)

**1. Integration Layer (4 tasks)**
- Create integrations module for external API calls [Use project backend framework]
- Implement service for specific external API (geocoding, data enrichment, etc.)
- Add HTTP client with retry logic and timeout (define retry count and timeout based on API SLA)
- Implement circuit breaker pattern to prevent cascading failures during API outages

**2. Business Logic (2 tasks)**
- Create IntegrationsService with methods for each external API integration
- Add response caching to reduce external API calls (define TTL based on data freshness requirements)

**3. API Layer (2 tasks)**
- Add endpoint to expose external API functionality (e.g., GET /api/integrations/[feature]) [Use project backend framework]
- Add comprehensive error handling for external API failures (graceful degradation)

**4. UI Layer (2 tasks)**
- Add UI control to trigger external API integration (button, auto-complete, etc.) [Use project frontend framework]
- Display loading states and user-friendly error messages for API calls

**5. Testing (3 tasks)**
- Add unit tests for integration service methods [Use project test framework]
- Add integration tests with mocked external API responses (success, failure, timeout scenarios)
- Add tests for circuit breaker and retry logic

**6. Documentation (1 task)**
- Update API documentation with integration endpoints and external API dependencies [Use project API doc tool]

### Key Considerations (adapt to project context)
- Research external API rate limits and implement appropriate throttling
- Implement circuit breaker (define failure threshold, e.g., 5 consecutive failures)
- Cache responses when appropriate (consider data freshness vs API costs)
- Always implement graceful degradation (allow manual input if API fails)
- Set timeouts to prevent hanging requests (typical: 5-10 seconds)
- Store API keys securely (environment variables, secrets management)
- Monitor external API health and costs

### Common Pitfalls
- Not implementing retry logic for transient failures (poor reliability)
- Missing circuit breaker (can cause cascading failures in your system)
- Not caching responses when appropriate (excessive API calls and costs)
- Not respecting external API rate limits (can lead to bans)
- Forgetting timeout configuration (hanging requests can exhaust resources)
- Exposing API keys in client-side code (security vulnerability)

---

## Refactoring Pattern

**Typical Story:** "As a developer I want to refactor [component/service] so that it follows best practices and is more maintainable"

### Task Breakdown (adapt to your tech stack)

**1. Preparatory Steps (2 tasks)**
- Add comprehensive tests for existing component behavior (ensures regression safety) [Use project test framework]
- Document current code structure and identify specific refactoring opportunities

**2. Refactoring (3-5 tasks depending on scope)**
- Extract separate concerns into focused modules/services/classes [Follow project architectural patterns]
- Refactor main component to use new extracted components with dependency injection
- Update module/container configuration with new dependencies [Use project framework DI pattern]
- Apply design patterns where appropriate (Strategy, Factory, etc.)

**3. Testing (2 tasks)**
- Run all existing tests to verify no regressions
- Add new tests for extracted components

**4. Cleanup (2 tasks)**
- Remove old commented code and dead code paths
- Update imports and references across the codebase

**5. Documentation (1 task)**
- Update architecture/technical documentation with new structure

### Key Considerations (adapt to project context)
- Refactoring should NOT change external behavior (keep same API/interface)
- All existing tests must pass after refactoring
- Consider feature flags for risky large-scale refactorings
- Incremental refactoring is safer than big bang rewrites
- Get code review for architectural changes
- Update documentation alongside code changes

### Common Pitfalls
- Refactoring without adequate test coverage (no regression safety - very risky)
- Changing behavior during refactoring (should be separate changes)
- Not updating all imports and references (broken builds)
- Refactoring too much at once (hard to review, high risk)
- Forgetting to update documentation (knowledge loss)
- Not communicating changes to team (breaks other developers' work)

---

## Pattern Selection Guide

Use this guide to select the appropriate pattern for your story:

| Story Type | Pattern |
|------------|---------|
| User registration, login, roles | Authentication & Authorization |
| Creating/viewing/editing entities | CRUD Operations |
| Purchasing content, handling payments | Payment Integration |
| Uploading files, documents, media | File Upload & Storage |
| Finding items by search/filters | Search & Filtering |
| Moderator approval workflows | Content Management |
| Calling external APIs, data enrichment | API Integration |
| Code restructuring, improving maintainability | Refactoring |

## General Task Guidelines

**For All Patterns:**
1. Always include testing tasks (unit, integration, e2e)
2. Update documentation after API changes
3. Consider security implications (authentication, authorization, input validation)
4. Add error handling and user-friendly error messages
5. Implement logging for debugging
6. Follow layer order: Data → Logic → API → UI → Tests → Docs

**Task Naming Best Practices:**
- Start with action verb (Create, Implement, Add, Build, Update)
- Specify layer or component (Prisma migration, NestJS service, Next.js page)
- Include key technical details (validation rules, constraints, libraries)
- Keep concise but descriptive (1 line)

**Technology References:**
Technology names should be discovered from your project's stack using the tech stack discovery process (README.md, package.json, requirements.txt, etc.). Reference `.opencode/templates/tech-stack.md` if you need to explicitly define your stack.

Use these patterns as starting points. Adapt to specific story requirements and your project's technology stack while maintaining consistency across the project.
