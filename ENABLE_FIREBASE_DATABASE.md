# Enable Firebase Realtime Database

The error "Not connected to Firebase Realtime Database" means the database needs to be activated.

## Steps to Enable:

1. **Go to Firebase Console**
   - Navigate to: https://console.firebase.google.com/project/videocall-signalling/database

2. **Create Database** (if not already created)
   - Click "Create Database" button
   - Choose location (United States is fine)
   - Start in "Test mode" for now (this gives 30 days of open access)
   - Click "Enable"

3. **Update Database Rules**
   After the database is created:
   - Go to the "Rules" tab
   - Replace the default rules with:
   ```json
   {
     "rules": {
       ".read": true,
       ".write": true
     }
   }
   ```
   - Click "Publish"

4. **Verify the Database URL**
   - In the "Data" tab, you should see the database URL at the top
   - It should be: `https://videocall-signalling-default-rtdb.firebaseio.com/`
   - If it's different, we need to update it in the code

## Alternative: If Realtime Database won't enable

If you see any errors or the Realtime Database option is not available:

1. Go to the Firebase Console home
2. Click on "All products"
3. Find "Realtime Database" and click on it
4. Click "Get started" or "Create Database"

## After Enabling

Once the database is enabled and rules are set:
1. Refresh the video call page
2. Check console for "Connected to Firebase Realtime Database"
3. The video call should now work

## Note
The database URL in your error message (`https://videocall-signalling-default-rtdb.firebaseio.com/`) looks correct, so the issue is most likely that the database hasn't been created yet or the rules are blocking access.