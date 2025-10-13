# Use the official Red Hat UBI8 Node.js 20 image
FROM registry.access.redhat.com/ubi9/nodejs-20

# Set the working directory
WORKDIR /opt/app-root/src

# Copy package.json
COPY package.json ./

# Install dependencies (skip audit fix due to permission restrictions in UBI9)
# Note: Security vulnerabilities should be fixed in package.json before building
RUN npm install --omit=dev && \
    npm cache clean --force

# Copy application source code
COPY . .

# Create a non-root user for security
USER 1001

# Expose the port the app runs on
EXPOSE 8080

# Set environment variables
ENV NODE_ENV=production
ENV PORT=8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/status || exit 1

# Start the application
CMD ["npm", "start"]
