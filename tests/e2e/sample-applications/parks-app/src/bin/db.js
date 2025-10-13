var config    = require('./config.js');
var db_svc    = config.get('db_svc_name'),
    db_export = {};

// Attempt to autoconfigure for PG and MongoDB
if( db_svc == "postgresql"){
  db_export = require('./pgdb.js');
}else if( db_svc == "mongodb"){
  db_export = require('./mongodb.js');
}else{
  console.log("ERROR: DB Configuration missing! Failed to autoconfigure database");
}

module.exports = exports = db_export;
