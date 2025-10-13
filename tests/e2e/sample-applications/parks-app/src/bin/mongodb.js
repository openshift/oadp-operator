var config      = require('./config.js'),
    { MongoClient } = require('mongodb'),
    path        = require('path');

var db_config   = config.get('db_config'),
    collection  = config.get('collection_name');

console.log("DB connection: " + db_config);

// Global variables for database connection
let client;
let db;

// Initialize MongoDB connection
async function initConnection() {
  try {
    client = new MongoClient(db_config);
    await client.connect();
    console.log('database connected');
    
    // Extract database name from connection string
    const dbName = db_config.split('/').pop().split('?')[0];
    db = client.db(dbName);
    
    return db;
  } catch (err) {
    console.log('database connection failed:', err.message);
    throw err;
  }
}

// Ensure connection is established with retry logic
async function ensureConnection() {
  if (!db) {
    const maxRetries = 5;
    const retryDelay = 2000; // 2 seconds
    
    for (let i = 0; i < maxRetries; i++) {
      try {
        await initConnection();
        return db;
      } catch (err) {
        console.log(`Database connection attempt ${i + 1}/${maxRetries} failed:`, err.message);
        if (i < maxRetries - 1) {
          console.log(`Retrying in ${retryDelay}ms...`);
          await new Promise(resolve => setTimeout(resolve, retryDelay));
        } else {
          throw new Error(`Failed to connect to database after ${maxRetries} attempts: ${err.message}`);
        }
      }
    }
  }
  return db;
}

async function init_db(persist_db_connection){
  try {
    const database = await ensureConnection();
    const coll = database.collection(collection);
    var points = require(path.resolve('./parkcoord.json'));
    
    // Create 2dsphere index (modern MongoDB uses 2dsphere instead of 2d)
    await coll.createIndex({'pos': "2dsphere"});
    console.log("index added on 'pos'");
    
    // Check if data already exists
    const count = await coll.countDocuments();
    if(count > 0){
      console.log("data already exists - bypassing db initialization work...");
      return persist_db_connection || client.close();
    } else {
      console.log("Importing map points...");
      await coll.insertMany(points);
      console.log("points imported");
      return persist_db_connection || client.close();
    }
  } catch(err) {
    console.log(err);
    return persist_db_connection || client.close();
  }
}

async function flush_db(persist_db_connection){
  try {
    console.log("Dropping the DB...");
    const database = await ensureConnection();
    const coll = database.collection(collection);
    await coll.drop();
    return persist_db_connection || client.close();
  } catch(err) {
    console.log(err);
    return persist_db_connection || client.close();
  }
} 

async function select_box(req, res, next){
  try {
    //clean these variables:
    var query = req.query;
    var lat1 = Number(query.lat1),
        lon1 = Number(query.lon1),
        lat2 = Number(query.lat2),
        lon2 = Number(query.lon2);
    var limit = (typeof(query.limit) !== "undefined") ? query.limit : 40;
    if(!(Number(query.lat1) 
      && Number(query.lon1) 
      && Number(query.lat2) 
      && Number(query.lon2)
      && Number(limit)))
    {
      res.send(500, {http_status:400,error_msg: "this endpoint requires two pair of lat, long coordinates: lat1 lon1 lat2 lon2\na query 'limit' parameter can be optionally specified as well."});
      return;
    }
    
    const database = await ensureConnection();
    const coll = database.collection(collection);
    
    // Use $geoWithin with $box for 2dsphere index
    const rows = await coll.find({
      "pos": {
        '$geoWithin': { 
          '$box': [[lon1,lat1],[lon2,lat2]]
        }
      }
    }).limit(parseInt(limit)).toArray();
    
    res.send(rows);
    return rows;
  } catch(err) {
    res.send(500, {http_status:500,error_msg: err});
    return console.error('error running query', err);
  }
};

async function select_all(req, res, next){
  try {
    const database = await ensureConnection();
    const coll = database.collection(collection);
    const rows = await coll.find({}).toArray();
    res.send(rows);
    return rows;
  } catch(err) {
    res.send(500, {http_status:500,error_msg: err});
    return console.error('error running query', err);
  }
};

// Click tracking functions
async function record_click(req, res, next){
  try {
    const database = await ensureConnection();
    const clicksCollection = database.collection('clicks');
    
    const clickData = {
      timestamp: new Date(),
      userAgent: req.headers['user-agent'] || 'unknown',
      ip: req.headers['x-forwarded-for'] || req.connection.remoteAddress || 'unknown'
    };
    
    await clicksCollection.insertOne(clickData);
    
    // Get total click count
    const totalClicks = await clicksCollection.countDocuments();
    
    res.send({ success: true, totalClicks: totalClicks });
    return { success: true, totalClicks: totalClicks };
  } catch(err) {
    res.send(500, {http_status:500, error_msg: err.message});
    return console.error('error recording click', err);
  }
};

async function get_click_count(req, res, next){
  try {
    const database = await ensureConnection();
    const clicksCollection = database.collection('clicks');
    const totalClicks = await clicksCollection.countDocuments();
    res.send({ totalClicks: totalClicks });
    return { totalClicks: totalClicks };
  } catch(err) {
    res.send(500, {http_status:500, error_msg: err.message});
    return console.error('error getting click count', err);
  }
};

module.exports = exports = {
  selectAll: select_all,
  selectBox: select_box,
  flushDB:   flush_db,
  initDB:    init_db,
  recordClick: record_click,
  getClickCount: get_click_count
};