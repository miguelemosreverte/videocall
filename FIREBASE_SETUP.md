# Firebase Realtime Database Setup

## Database Rules

To allow read/write access for the video call app, you need to update your Firebase Realtime Database rules.

Go to your Firebase Console:
1. Navigate to https://console.firebase.google.com/project/videocall-signalling/database
2. Click on the "Rules" tab
3. Replace the existing rules with:

```json
{
  "rules": {
    ".read": true,
    ".write": true,
    "peers": {
      "$uid": {
        ".write": true,
        ".read": true
      }
    },
    "messages": {
      "$uid": {
        ".write": true,
        ".read": true,
        "$messageId": {
          ".write": true
        }
      }
    }
  }
}
```

4. Click "Publish" to save the rules

## Security Note

These rules allow anyone to read/write to your database. For production use, you should implement proper authentication and security rules.

## Testing the Connection

After updating the rules, refresh the video call app. You should see in the console:
- "Firebase initialized successfully"
- "Connected to Firebase Realtime Database"

If you still see errors, check:
1. The database URL is correct
2. The Firebase project is active
3. The Realtime Database is enabled in your Firebase project