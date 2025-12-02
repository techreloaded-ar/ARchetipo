# Task Breakdown Patterns

This document provides pattern-based guidance for breaking down user stories into executable technical tasks. Use these patterns to ensure consistent, comprehensive task planning across the MotorRider project.

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

**Typical Story:** "As a biker I want to register and login so that I can purchase itineraries"

### Task Breakdown (MotorRider tech stack)

**1. Data Layer (3 tasks)**
- Create Prisma migration for User model (email, passwordHash, role enum [BIKER/RANGER/ADMIN], createdAt, updatedAt)
- Define User entity in Prisma schema with unique email constraint and indexes
- Generate Prisma client and run migration on dev database

**2. Business Logic (4 tasks)**
- Create NestJS AuthModule with bcrypt dependency (12 rounds salt)
- Implement AuthService.register() with email uniqueness check and password hashing
- Implement AuthService.login() with JWT generation (expiry: 7 days)
- Implement password reset service with token generation and expiry (1 hour)

**3. API Layer (4 tasks)**
- Create AuthController with POST /api/auth/register (DTO: email, password min 8 chars)
- Add POST /api/auth/login endpoint with rate limiting (5 attempts/min per IP using @nestjs/throttler)
- Add POST /api/auth/reset-password endpoint
- Create JwtAuthGuard for route protection and RolesGuard for RBAC

**4. UI Layer (3 tasks)**
- Build Next.js /register page with React Hook Form and Tailwind CSS
- Build Next.js /login page with error handling and redirect logic
- Build /forgot-password page with email validation

**5. Testing (4 tasks)**
- Add Jest unit tests for User Prisma model validation (email format, unique constraint)
- Add Jest unit tests for AuthService methods (register, login, token generation)
- Add integration tests for auth endpoints using Supertest (201 success, 400/409 errors)
- Add e2e tests for registration and login flows using Playwright

**6. Documentation (1 task)**
- Update Swagger/OpenAPI spec with auth endpoints, DTOs, and response schemas

### Key Considerations (MotorRider context)
- Role enum must default to BIKER on registration
- JWT payload includes userId and role for authorization checks
- Password requirements: min 8 chars, at least 1 uppercase, 1 number (per NFR)
- Rate limiting prevents brute force on login endpoint
- Secure session via JWT stored in HttpOnly cookie (CSRF protection)
- Password reset link sent via email service integration
- GDPR: user consent stored separately, audit log for auth events

### Common Pitfalls
- Forgetting to hash password before Prisma User.create()
- Not handling duplicate email error (Prisma P2002 code)
- Missing rate limiting on /login (security vulnerability)
- Storing JWT in localStorage (XSS risk - use HttpOnly cookie instead)
- Not testing concurrent registration attempts
- Forgetting to validate role enum values

---

## CRUD Operations Pattern

**Typical Story:** "As a ranger I want to create itineraries so that bikers can purchase them"

### Task Breakdown (MotorRider tech stack)

**1. Data Layer (3 tasks)**
- Create Prisma migration for Itinerary model (title, description, difficulty, region, distance, duration, price, status enum [DRAFT/PENDING/APPROVED/REJECTED], rangerId FK, createdAt, updatedAt)
- Define Itinerary entity in Prisma schema with relations to User (ranger) and indexes on status, region
- Generate Prisma client and run migration

**2. Business Logic (4 tasks)**
- Create NestJS ItinerariesModule
- Implement ItinerariesService with CRUD methods (create, findAll, findOne, update, delete)
- Add business logic for status transitions (DRAFT → PENDING → APPROVED/REJECTED)
- Implement ownership validation (rangers can only edit their own itineraries)

**3. API Layer (5 tasks)**
- Create ItinerariesController with POST /api/itineraries (create, requires RANGER role)
- Add GET /api/itineraries (list all, with pagination and filtering)
- Add GET /api/itineraries/:id (get single)
- Add PATCH /api/itineraries/:id (update, ownership check)
- Add DELETE /api/itineraries/:id (soft delete, ownership check)

**4. UI Layer (4 tasks)**
- Build Next.js /itineraries page with list, pagination, and filters
- Build Next.js /itineraries/new page for rangers (create form)
- Build Next.js /itineraries/:id page (detail view with conditional edit button)
- Build Next.js /itineraries/:id/edit page for rangers (edit form)

**5. Testing (4 tasks)**
- Add Jest unit tests for Itinerary model validation
- Add Jest unit tests for ItinerariesService CRUD operations
- Add integration tests for itineraries endpoints (CRUD operations, authorization checks)
- Add e2e tests for complete create/view/edit/delete flows

**6. Documentation (1 task)**
- Update Swagger/OpenAPI spec with itineraries endpoints and DTOs

### Key Considerations (MotorRider context)
- Status workflow enforces quality: rangers create DRAFT, admin approves to APPROVED
- Only APPROVED itineraries visible to bikers in catalog
- Soft delete preserves data integrity for purchased itineraries
- Price stored in cents (integer) to avoid floating-point precision issues
- Pagination required (default 20 items per page)
- Filtering by region, difficulty, price range

### Common Pitfalls
- Not implementing pagination (performance issue with large datasets)
- Forgetting ownership checks (security vulnerability)
- Hard delete instead of soft delete (breaks purchase history)
- Not validating status transitions (invalid workflows)
- Missing indexes on frequently queried fields (performance)

---

## Payment Integration Pattern

**Typical Story:** "As a biker I want to purchase an itinerary so that I can access premium content and download GPX"

### Task Breakdown (MotorRider tech stack)

**1. Data Layer (3 tasks)**
- Create Prisma migration for Purchase model (userId FK, itineraryId FK, amount, currency, paymentProvider enum [STRIPE/PAYPAL], paymentIntentId, status enum [PENDING/COMPLETED/FAILED/REFUNDED], createdAt, completedAt)
- Define Purchase entity with relations to User and Itinerary, unique constraint on (userId, itineraryId)
- Generate Prisma client and run migration

**2. Business Logic (5 tasks)**
- Create NestJS PaymentsModule with Stripe SDK integration
- Implement PaymentsService.createPaymentIntent() for Stripe payment creation
- Implement PaymentsService.handleStripeWebhook() for payment status updates
- Create PurchasesService to track purchases and unlock content
- Add logic to calculate ranger revenue share (50/50 split)

**3. API Layer (4 tasks)**
- Create PaymentsController with POST /api/payments/intent (create Stripe PaymentIntent)
- Add POST /api/webhooks/stripe (handle payment events: payment_intent.succeeded)
- Create PurchasesController with GET /api/purchases (user's purchase history)
- Add GET /api/purchases/:itineraryId/access (check if user has access)

**4. UI Layer (4 tasks)**
- Build Next.js /itineraries/:id/purchase page with Stripe Elements integration
- Add payment confirmation UI with success/error states
- Build Next.js /my-purchases page showing user's purchased itineraries
- Add conditional "Download GPX" button on itinerary detail (only if purchased)

**5. Integration Layer (3 tasks)**
- Configure Stripe webhook endpoint with signature verification
- Implement idempotency handling for webhook retry scenarios
- Add webhook event logging and error monitoring

**6. Testing (5 tasks)**
- Add Jest unit tests for payment intent creation logic
- Add Jest unit tests for webhook handler (payment status transitions)
- Add integration tests for payment endpoints with Stripe test mode
- Add e2e tests for complete purchase flow (payment → webhook → content unlock)
- Add tests for idempotency and duplicate webhook handling

**7. Documentation (1 task)**
- Update Swagger/OpenAPI spec with payment endpoints and webhook format

### Key Considerations (MotorRider context)
- Use Stripe PaymentIntents API for SCA compliance (PSD2)
- Webhook signature verification prevents spoofing
- Idempotency keys prevent duplicate charges on retries
- Store amount in cents to avoid precision errors
- Unique constraint on (userId, itineraryId) prevents duplicate purchases
- Revenue share calculation: 50% to ranger, 50% to platform
- Handle webhook retries gracefully (Stripe retries up to 3 days)

### Common Pitfalls
- Not verifying webhook signatures (security vulnerability)
- Missing idempotency handling (duplicate charges)
- Storing price in float (precision errors)
- Not handling payment failures gracefully
- Forgetting to test webhook retry scenarios
- Not logging payment events for debugging

---

## File Upload & Storage Pattern

**Typical Story:** "As a ranger I want to upload GPX tracks and photos so that itineraries have navigation data and visuals"

### Task Breakdown (MotorRider tech stack)

**1. Data Layer (3 tasks)**
- Create Prisma migration for Media model (type enum [GPX/PHOTO], fileName, s3Key, s3Bucket, fileSize, mimeType, itineraryId FK, uploadedBy FK, createdAt)
- Define Media entity with relations to Itinerary and User
- Generate Prisma client and run migration

**2. Storage Layer (4 tasks)**
- Configure S3-compatible storage client (AWS SDK or MinIO client)
- Implement StorageService.uploadFile() with multipart upload support
- Implement StorageService.generateSignedUrl() for secure downloads (expiry: 1 hour)
- Implement StorageService.deleteFile() for cleanup

**3. Business Logic (3 tasks)**
- Create NestJS MediaModule
- Implement MediaService with upload validation (file type, size limits: GPX max 5MB, photos max 10MB)
- Implement MediaService.createMediaRecord() to track uploads in database

**4. API Layer (4 tasks)**
- Create MediaController with POST /api/media/upload (multipart/form-data, requires authentication)
- Add validation interceptor for file type and size
- Add GET /api/media/:id/download (generates signed URL, requires purchase check for GPX)
- Add DELETE /api/media/:id (soft delete, ownership check)

**5. UI Layer (3 tasks)**
- Build Next.js file upload component with drag-and-drop (using react-dropzone)
- Add upload progress indicator
- Build media gallery component for itinerary detail page

**6. Testing (4 tasks)**
- Add Jest unit tests for file validation logic
- Add Jest unit tests for signed URL generation
- Add integration tests for upload endpoint (success, size limit, type validation)
- Add e2e tests for complete upload and download flow

**7. Documentation (1 task)**
- Update Swagger/OpenAPI spec with media upload endpoints

### Key Considerations (MotorRider context)
- GPX files require validation (valid XML structure, coordinates)
- Photos optimized on upload (resize, compression)
- Signed URLs expire after 1 hour to prevent sharing
- GPX downloads only for users who purchased the itinerary
- S3 bucket configured with private ACL (no public access)
- Virus scanning recommended for uploaded files

### Common Pitfalls
- Not validating file types (security risk)
- Missing file size limits (storage costs, DoS risk)
- Using public S3 URLs (bypasses purchase check)
- Not cleaning up orphaned files (storage waste)
- Forgetting to handle upload failures
- Not optimizing large images (bandwidth costs)

---

## Search & Filtering Pattern

**Typical Story:** "As a biker I want to search and filter itineraries so that I can find routes matching my preferences"

### Task Breakdown (MotorRider tech stack)

**1. Data Layer (2 tasks)**
- Add database indexes on filterable fields (region, difficulty, price, distance, duration)
- Create full-text search index on Itinerary title and description (PostgreSQL tsvector)

**2. Business Logic (3 tasks)**
- Extend ItinerariesService with search() method using Prisma filters
- Implement QueryBuilder for dynamic WHERE clauses based on filters
- Add sorting logic (price, distance, popularity, createdAt)

**3. API Layer (2 tasks)**
- Extend GET /api/itineraries with query params (q, region, difficulty, minPrice, maxPrice, minDistance, maxDistance, sort, page, limit)
- Add response metadata (totalCount, currentPage, totalPages, hasNextPage)

**4. UI Layer (4 tasks)**
- Build Next.js search bar component with debounced input
- Build filter sidebar component (region dropdown, difficulty checkboxes, price/distance sliders)
- Add sorting dropdown (price, distance, newest)
- Implement pagination controls

**5. Testing (3 tasks)**
- Add Jest unit tests for QueryBuilder logic
- Add integration tests for search endpoint (full-text search, filters, sorting, pagination)
- Add e2e tests for complete search and filter flow

**6. Documentation (1 task)**
- Update Swagger/OpenAPI spec with search query parameters

### Key Considerations (MotorRider context)
- Full-text search on title and description using PostgreSQL tsvector
- Debounce search input (300ms) to reduce API calls
- Pagination required (default 20 items per page)
- Filters are ANDed together (region AND difficulty AND price range)
- Default sort: newest first
- Cache search results for 5 minutes (Redis)

### Common Pitfalls
- Not debouncing search input (excessive API calls)
- Missing indexes on filterable fields (slow queries)
- Not implementing pagination (performance issue)
- Forgetting to validate query parameters
- Not handling empty results gracefully

---

## Content Management Pattern

**Typical Story:** "As an admin I want to review and approve ranger-submitted itineraries so that only quality content is published"

### Task Breakdown (MotorRider tech stack)

**1. Data Layer (2 tasks)**
- Add status transitions audit log (statusHistory JSONB field in Itinerary)
- Create Prisma migration for admin approval workflow

**2. Business Logic (3 tasks)**
- Extend ItinerariesService with admin methods (approve, reject, requestChanges)
- Add status transition validation (PENDING → APPROVED/REJECTED only by admin)
- Implement email notifications for rangers on status changes

**3. API Layer (3 tasks)**
- Add PATCH /api/admin/itineraries/:id/approve endpoint (ADMIN role required)
- Add PATCH /api/admin/itineraries/:id/reject endpoint with reason field
- Add GET /api/admin/itineraries/pending (list itineraries awaiting review)

**4. UI Layer (3 tasks)**
- Build Next.js /admin/itineraries page with pending queue
- Build itinerary review modal with approve/reject buttons and reason textarea
- Add status badges (DRAFT/PENDING/APPROVED/REJECTED) to itinerary cards

**5. Testing (3 tasks)**
- Add Jest unit tests for status transition logic
- Add integration tests for admin endpoints (authorization checks)
- Add e2e tests for complete approval workflow

**6. Documentation (1 task)**
- Update Swagger/OpenAPI spec with admin endpoints

### Key Considerations (MotorRider context)
- Only ADMIN role can approve/reject itineraries
- Email notification sent to ranger on status change
- Rejection reason required (helps rangers improve)
- Status history logged for audit trail
- DRAFT itineraries not visible to admins (only PENDING)

### Common Pitfalls
- Not enforcing ADMIN role (security vulnerability)
- Missing email notifications (poor UX)
- Forgetting to log status changes (audit trail)
- Allowing invalid status transitions
- Not providing rejection reasons

---

## API Integration Pattern

**Typical Story:** "As a ranger I want to auto-populate itinerary details from external sources so that I can save time"

### Task Breakdown (MotorRider tech stack)

**1. Integration Layer (4 tasks)**
- Create NestJS IntegrationsModule for external API calls
- Implement OpenStreetMap Nominatim service for geocoding (address → coordinates)
- Add HTTP client with retry logic and timeout (3 retries, 5s timeout)
- Implement circuit breaker pattern to prevent cascading failures

**2. Business Logic (2 tasks)**
- Create IntegrationsService with methods for each external API
- Add response caching (Redis, TTL: 24 hours) to reduce API calls

**3. API Layer (2 tasks)**
- Add GET /api/integrations/geocode endpoint (query param: address)
- Add error handling for external API failures (graceful degradation)

**4. UI Layer (2 tasks)**
- Add "Auto-fill from address" button on itinerary form
- Display loading state and error messages for API calls

**5. Testing (3 tasks)**
- Add Jest unit tests for integration service methods
- Add integration tests with mocked external API responses
- Add tests for circuit breaker and retry logic

**6. Documentation (1 task)**
- Update Swagger/OpenAPI spec with integration endpoints

### Key Considerations (MotorRider context)
- OSM Nominatim has rate limiting (1 req/sec)
- Circuit breaker opens after 5 consecutive failures
- Cache responses to reduce external API calls
- Graceful degradation: show error, allow manual input
- Timeout prevents hanging requests

### Common Pitfalls
- Not implementing retry logic (transient failures)
- Missing circuit breaker (cascading failures)
- Not caching responses (excessive API calls)
- Not handling rate limiting from external APIs
- Forgetting timeout on HTTP requests

---

## Refactoring Pattern

**Typical Story:** "As a developer I want to refactor the authentication service so that it follows SOLID principles"

### Task Breakdown (MotorRider tech stack)

**1. Preparatory Steps (2 tasks)**
- Add comprehensive tests for existing AuthService behavior (regression safety)
- Document current code structure and identify refactoring opportunities

**2. Refactoring (3-5 tasks depending on scope)**
- Extract password hashing logic into separate PasswordService
- Extract JWT logic into separate TokenService
- Refactor AuthService to use injected dependencies
- Update AuthModule with new service providers

**3. Testing (2 tasks)**
- Run existing tests to verify no regressions
- Add new tests for extracted services

**4. Cleanup (2 tasks)**
- Remove old commented code
- Update imports across the codebase

**5. Documentation (1 task)**
- Update architecture docs with new service structure

### Key Considerations (MotorRider context)
- Refactoring should not change external behavior
- All existing tests must pass
- Deploy behind feature flag if risky
- Incremental refactoring preferred over big bang

### Common Pitfalls
- Refactoring without tests (no regression safety)
- Changing behavior during refactoring
- Not updating imports (broken builds)
- Refactoring too much at once
- Forgetting to update documentation

---

## Pattern Selection Guide

Use this guide to select the appropriate pattern for your story:

| Story Type | Pattern |
|------------|---------|
| User registration, login, roles | Authentication & Authorization |
| Creating/viewing/editing entities | CRUD Operations |
| Buying itineraries, handling payments | Payment Integration |
| Uploading GPX, photos | File Upload & Storage |
| Finding itineraries by filters | Search & Filtering |
| Admin approval workflows | Content Management |
| Calling external APIs | API Integration |
| Code restructuring | Refactoring |

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

**Technology References (MotorRider stack):**
- Frontend: Next.js, React, React Hook Form, Tailwind CSS
- Backend: NestJS, Node.js
- Database: PostgreSQL, Prisma ORM
- Storage: S3-compatible (AWS S3 or MinIO)
- Payments: Stripe, PayPal
- Maps: OpenStreetMap (OSM)
- Testing: Jest, Supertest, Playwright
- Auth: JWT, bcrypt
- API Docs: Swagger/OpenAPI

Use these patterns as starting points. Adapt to specific story requirements while maintaining consistency across the project.
