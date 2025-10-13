const restify = require('restify'),
      fs      = require('fs'),
      config  = require('./bin/config.js'),
      db      = require('./bin/db.js');
const app     = restify.createServer();

// Initialize database asynchronously (will retry when MongoDB is available)
db.initDB('keepAlive').catch(err => {
  console.error('Database initialization failed (MongoDB may not be ready yet):', err.message);
  console.log('Application will continue to start and retry database connection...');
});

app.use(restify.plugins.queryParser())
app.use(restify.plugins.bodyParser())
app.use(restify.plugins.fullResponse())

// CORS handling for Restify v11
app.use(function(req, res, next) {
  res.header('Access-Control-Allow-Origin', '*');
  res.header('Access-Control-Allow-Headers', 'X-Requested-With, Content-Type, Authorization');
  res.header('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS');
  if (req.method === 'OPTIONS') {
    res.send(200);
  } else {
    next();
  }
})

// Routes - wrap async functions for Restify v11
app.get('/parks/within', async (req, res) => {
  try {
    await db.selectBox(req, res);
  } catch (err) {
    res.send(500, { error: err.message });
  }
});

app.get('/parks', async (req, res) => {
  try {
    await db.selectAll(req, res);
  } catch (err) {
    res.send(500, { error: err.message });
  }
});
app.get('/status', (req, res, next) => {
  res.send({ status: 'ok' });
  next();
});

// Click tracking endpoints
app.post('/clicks', async (req, res) => {
  try {
    await db.recordClick(req, res);
  } catch (err) {
    res.send(500, { error: err.message });
  }
});

app.get('/clicks', async (req, res) => {
  try {
    await db.getClickCount(req, res);
  } catch (err) {
    res.send(500, { error: err.message });
  }
});

app.get('/', (req, res, next) => {
  try {
    const data = fs.readFileSync(__dirname + '/index.html');
    res.status(200);
    res.header('Content-Type', 'text/html');
    res.end(data.toString().replace(/host:port/g, req.header('Host')));
    next();
  } catch (err) {
    res.send(500, { error: 'Failed to load index.html' });
    next();
  }
});

// Static file serving for Restify v11 - using middleware approach
app.use((req, res, next) => {
  // Check if this is a static file request
  if (req.url.match(/^\/(css|js|img)\//)) {
    const path = require('path');
    const filePath = path.join(__dirname, 'static', req.url);
    
    try {
      const data = fs.readFileSync(filePath);
      const ext = path.extname(filePath);
      const contentType = {
        '.css': 'text/css',
        '.js': 'application/javascript',
        '.jpg': 'image/jpeg',
        '.png': 'image/png',
        '.gif': 'image/gif'
      }[ext] || 'application/octet-stream';
      
      res.header('Content-Type', contentType);
      res.send(200, data);
      return; // Don't call next() for static files
    } catch (err) {
      res.send(404, { error: 'File not found' });
      return; // Don't call next() for static files
    }
  }
  
  // Not a static file, continue to next middleware
  next();
});

app.listen(config.get('PORT'), config.get('IP'), () => {
  console.log(`Listening on ${config.get('IP')}, port ${config.get('PORT')}`);
});
