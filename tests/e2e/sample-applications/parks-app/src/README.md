# Parks App - National Parks and Historic Sites
*Modern mapping application powered by Node.js, MongoDB 7.0, and Leaflet maps*

A modern mapping application that visualizes the locations of major National Parks and Historic Sites using MongoDB 7.0, Restify v11, and Leaflet maps.

## Features

- **Modern Tech Stack**: Node.js 20, MongoDB 7.0, Restify v11
- **Interactive Maps**: Leaflet-based mapping with National Parks data
- **RESTful API**: Clean API endpoints for park data
- **Container Ready**: Optimized Dockerfile for OpenShift/Kubernetes
- **Security Hardened**: Non-root user, modern dependencies

## Quick Start

### Using Docker/Podman:
```bash
# Build the image
podman build -t parksapp .

# Run the container
podman run -d --name parksapp -p 8080:8080 parksapp
```

### Using OpenShift:
```bash
# Create project
oc new-project parks-app

# Deploy from source
oc new-app https://github.com/weshayutin/mig-demo-apps#parks_app_fix --context-dir=apps/parks-app/src
```

## API Endpoints

- `GET /` - Main application interface
- `GET /parks` - Get all parks
- `GET /parks/within?lat1=...&lon1=...&lat2=...&lon2=...` - Get parks within bounding box
- `GET /status` - Health check endpoint

## Environment Variables

- `PORT` - Server port (default: 8080)
- `MONGODB_DB_URL` - MongoDB connection string
- `MONGODB_DATABASE` - Database name (default: restify-database)

## Local Development

### Prerequisites
- Node.js 20+ 
- MongoDB 7.0+
- Git

### Setup
```bash
# Clone the repository
git clone https://github.com/weshayutin/mig-demo-apps.git
cd mig-demo-apps/apps/parks-app/src

# Install dependencies
npm install

# Set environment variables
export MONGODB_DB_URL="mongodb://localhost:27017"
export MONGODB_DATABASE="restify-database"

# Start the application
npm start
```

The application will be available at `http://localhost:8080`

## Database Setup

The application automatically initializes the database with National Parks data on first startup. The database will contain approximately 547 park records.

### MongoDB Connection
The application uses the following MongoDB configuration:
- **Database**: `restify-database`
- **Collection**: `parks`
- **Index**: 2dsphere index on `pos` field for geospatial queries

### Testing Database Connection
You can verify the database connection using the MongoDB shell:
```bash
mongosh mongodb://localhost:27017/restify-database
```

Check the number of records:
```javascript
db.parks.countDocuments()
// Should return 547
```

## Deployment

### Using Git
To deploy updates to your application:

1. Add your changes:
   ```bash
   git add .
   ```

2. Commit your changes:
   ```bash
   git commit -m "Describe your changes"
   ```

3. Push to trigger a new build:
   ```bash
   git push
   ```

### Using OpenShift CLI
```bash
# Start a new build
oc start-build restify

# Check build status
oc get builds

# View build logs
oc logs -f build/restify-<build-number>
```

## License
MIT License
