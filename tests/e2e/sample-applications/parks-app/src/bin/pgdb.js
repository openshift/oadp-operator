var config     = require('./config.js'),
    pg         = require('pg-query'),
    path       = require('path');
var table_name = config.get('table_name');

pg.connectionParameters = config.get('db_config');
console.log(pg.connectionParameters);

var error_response = "data already exists - bypassing db initialization step\n";

function createDBSchema(err, rows, result) {
  if(err && err.code == "ECONNREFUSED"){
    return console.error("DB connection unavailable, see README notes for setup assistance\n", err);
  }
  var query = "CREATE TABLE "+table_name+" ( gid serial NOT NULL, name character varying(240), the_geom geometry, CONSTRAINT "+table_name+ "_pkey PRIMARY KEY (gid), CONSTRAINT enforce_dims_geom CHECK (st_ndims(the_geom) = 2), CONSTRAINT enforce_geotype_geom CHECK (geometrytype(the_geom) = 'POINT'::text OR the_geom IS NULL),CONSTRAINT enforce_srid_geom CHECK (st_srid(the_geom) = 4326) ) WITH ( OIDS=FALSE );";
  pg(query, addSpatialIndex);
};

function addSpatialIndex(err, rows, result) {
  if(err) {
    return console.error(error_response, err);
  }
  pg("CREATE INDEX "+table_name+"_geom_gist ON "+table_name+" USING gist (the_geom);", importMapPoints);
}

function importMapPoints(err, rows, result) {
  if(err) {
    return console.error(error_response, err);
  }
  var points = require(path.resolve('./parkcoord.json'));
  var insert = "Insert into "+table_name+" (name, the_geom) VALUES ";
  var qpoints = points.map(insertMapPinSQL).join(",");
  var query = insert + qpoints + ';';
  console.log(query);
  pg(query, function(err, rows, result) {
    if(err) {
      return console.error(error_response, err);
    }
    var response = 'Data import completed!';
    return response;
  });
};

function insertMapPinSQL(pin) {
  var query = '';
  var escape = /'/g
  
  if(typeof(pin) == 'object'){
    query = "('" + pin.Name.replace(/'/g,"''") + "', ST_GeomFromText('POINT(" + pin.pos[0] +" "+ pin.pos[1] + " )', 4326))";  
  }
  return query;
};

function init_db(){
  pg('CREATE EXTENSION postgis;', createDBSchema);
} 

function flush_db(){
  pg('DROP TABLE '+ table_name+';', function(err, rows, result){
    var response = 'Database dropped!';
    console.log(response);
    return response;
  });
} 

function select_box(req, res, next){
  //clean these variables:
  var query = req.query;
  var limit = (typeof(query.limit) !== "undefined") ? query.limit : 40;
  if(!(Number(query.lat1) 
    && Number(query.lon1) 
    && Number(query.lat2) 
    && Number(query.lon2)
    && Number(limit)))
  {
    res.send(500, {http_status:400,error_msg: "this endpoint requires two pair of lat, long coordinates: lat1 lon1 lat2 lon2\na query 'limit' parameter can be optionally specified as well."});
    return console.error('could not connect to postgres', err);
  }
  pg('SELECT gid,name,ST_X(the_geom) as lon,ST_Y(the_geom) as lat FROM ' + table_name+ ' t WHERE ST_Intersects( ST_MakeEnvelope('+query.lon1+", "+query.lat1+", "+query.lon2+", "+query.lat2+", 4326), t.the_geom) LIMIT "+limit+';', function(err, rows, result){
    if(err) {
      res.send(500, {http_status:500,error_msg: err})
      return console.error('error running query', err);
    }
    res.send(rows);
    return rows;
  })
};
function select_all(req, res, next){
  console.log(pg);
  pg('SELECT gid,name,ST_X(the_geom) as lon,ST_Y(the_geom) as lat FROM ' + table_name +';', function(err, rows, result) {
    console.log(config);
    if(err) {
      res.send(500, {http_status:500,error_msg: err})
      return console.error('error running query', err);
    }
    res.send(result);
    return rows;
  });
};

module.exports = exports = {
  selectAll: select_all,
  selectBox: select_box,
  flushDB:   flush_db,
  initDB:    init_db
};
