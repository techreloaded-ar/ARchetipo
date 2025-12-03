# Tech Stack Configuration

Use this file to explicitly define your project's technology stack. The architect-agent will prioritize this file over automatic discovery, allowing you to override or supplement auto-detected technologies.

## Frontend
- **Framework:** [e.g., Next.js, Vue, Angular, React, Svelte]
- **UI Library:** [e.g., Tailwind CSS, Material UI, Bootstrap, Ant Design, Chakra UI]
- **Form Handling:** [e.g., React Hook Form, Formik, Vuelidate, Angular Forms]
- **State Management:** [e.g., Redux, Zustand, Pinia, NgRx, MobX] (if applicable)

## Backend
- **Framework:** [e.g., NestJS, Express, Django, Flask, FastAPI, Spring Boot, Ruby on Rails]
- **Language:** [e.g., TypeScript, JavaScript, Python, Java, Kotlin, Ruby, Go, C#]
- **API Style:** [e.g., REST, GraphQL, gRPC, tRPC]

## Database
- **Database:** [e.g., PostgreSQL, MySQL, MongoDB, SQL Server, Oracle]
- **ORM/ODM:** [e.g., Prisma, TypeORM, Sequelize, SQLAlchemy, Hibernate, Mongoose, Entity Framework]

## Storage
- **Provider:** [e.g., AWS S3, MinIO, Azure Blob Storage, Google Cloud Storage, Cloudinary]
- **Use Case:** [e.g., User uploads, static assets, backups]

## Authentication
- **Method:** [e.g., JWT, Session-based, OAuth 2.0, SAML]
- **Library:** [e.g., Passport.js, NextAuth, Auth0, Keycloak, Django Auth, Spring Security]

## Testing
- **Unit Tests:** [e.g., Jest, Vitest, Pytest, JUnit, xUnit, Mocha]
- **Integration Tests:** [e.g., Supertest, TestClient (FastAPI), REST Assured, Postman]
- **E2E Tests:** [e.g., Playwright, Cypress, Selenium, Puppeteer]

## Additional Services (if applicable)

### Payment Processing
- **Provider:** [e.g., Stripe, PayPal, Square, Braintree, Adyen]

### Email
- **Provider:** [e.g., SendGrid, AWS SES, Mailgun, Postmark, Resend]

### Maps/Geolocation
- **Provider:** [e.g., Google Maps, OpenStreetMap, Mapbox, Azure Maps]

### Real-time Communication
- **Technology:** [e.g., WebSockets, Socket.io, Pusher, Ably, SignalR]

### Caching
- **Technology:** [e.g., Redis, Memcached, in-memory cache]

### Queue/Background Jobs
- **Technology:** [e.g., Bull, BullMQ, Celery, Sidekiq, AWS SQS, RabbitMQ]

### Monitoring/Logging
- **APM:** [e.g., Datadog, New Relic, Sentry, Application Insights]
- **Logging:** [e.g., Winston, Pino, Loguru, Log4j, Serilog]

### API Documentation
- **Tool:** [e.g., Swagger/OpenAPI, Redoc, API Blueprint, Postman Collections]

## Deployment

### Hosting
- **Platform:** [e.g., Vercel, AWS, Azure, Google Cloud, Heroku, DigitalOcean, Railway]

### Containerization
- **Technology:** [e.g., Docker, Kubernetes, Docker Compose]

### CI/CD
- **Pipeline:** [e.g., GitHub Actions, GitLab CI, CircleCI, Jenkins, Azure DevOps]

---

## Instructions

1. **Fill in the technologies used in your project** by replacing the bracketed examples with your actual stack
2. **Remove sections that don't apply** to your project
3. **Add custom sections** if your project uses technologies not listed here
4. **Be specific** - include version numbers if certain features depend on specific versions

The architect-agent will read this file during tech stack discovery and use these technologies when generating task breakdowns.
