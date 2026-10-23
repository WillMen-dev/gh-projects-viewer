// Se ejecuta UNA sola vez, cuando el volumen de MongoDB está vacío.
// Crea un usuario de aplicación con permiso readWrite SOLO sobre la base de la app.
const dbName = process.env.MONGO_INITDB_DATABASE || 'gh_projects_viewer';
const appUser = process.env.MONGO_APP_USER;
const appPass = process.env.MONGO_APP_PASSWORD;

if (!appUser || !appPass) {
  throw new Error('MONGO_APP_USER y MONGO_APP_PASSWORD son obligatorios');
}

db.getSiblingDB(dbName).createUser({
  user: appUser,
  pwd: appPass,
  roles: [{ role: 'readWrite', db: dbName }],
});
print(`Usuario de aplicación '${appUser}' creado en '${dbName}'`);
