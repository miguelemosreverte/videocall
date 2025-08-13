# FCM vs Realtime Database for WebRTC Signaling

## Why Realtime Database is Better for WebRTC Signaling

### FCM (Firebase Cloud Messaging) Limitations:
1. **One-way communication**: FCM is designed for server-to-client push notifications
2. **Requires backend server**: You need a server to send FCM messages (can't send directly browser-to-browser)
3. **No real-time bidirectional**: Browsers can't send FCM messages to each other
4. **Latency**: FCM is optimized for reliability over speed
5. **No presence system**: Can't easily track who's online
6. **Token management**: Need to manage FCM tokens for each client

### Realtime Database Advantages:
1. **Bidirectional**: Browsers can read and write directly
2. **No server needed**: Pure peer-to-peer signaling
3. **Real-time listeners**: Instant message delivery
4. **Presence system**: Built-in `.info/connected` for online status
5. **Simple setup**: Just need database rules
6. **Perfect for WebRTC**: Designed for real-time data sync

## When to Use FCM:
- Mobile app notifications
- Background updates
- One-way server announcements
- Waking up dormant apps

## When to Use Realtime Database:
- WebRTC signaling (our use case)
- Real-time collaboration
- Presence/online status
- Chat applications
- Live data synchronization

## Conclusion:
For WebRTC video calling in a web browser, Realtime Database is the correct choice because:
- Browsers can't receive FCM push messages directly
- We need bidirectional signaling (offers, answers, ICE candidates)
- We need real-time presence detection
- No backend server required