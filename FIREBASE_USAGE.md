# Firebase Usage Monitoring

## Where to Check Usage
**Main Dashboard**: https://console.firebase.google.com/u/0/project/videocall-signalling/overview

## Key Metrics to Monitor

### Realtime Database Usage
- Navigate to: **Realtime Database → Usage tab**
- Monitor:
  - **Downloads (Bandwidth)**: X GB / 10 GB monthly limit
  - **Storage**: X KB / 1 GB limit  
  - **Concurrent Connections**: X / 100 limit

### Current Usage (as of last check)
- **Downloads (7d total)**: 745 KB
- **Storage (current)**: 11.5 KB
- **Status**: Well within free tier limits (< 0.1% usage)

## Free Tier (Spark Plan) Limits
- **10 GB/month** bandwidth (downloads)
- **1 GB** stored data
- **100** simultaneous connections
- **No billing account** = Cannot be charged (app stops at limits)

## Usage Estimates for Video Calls
- **Current implementation**: HD video (1280x720) at 25 FPS
- **Expected usage**: ~2.5 MB/sec per user
- **Two users, 1 hour**: ~18 GB
- **Monthly reset**: 1st of each month

## Important Notes
- Without billing account linked, service stops at limits (no charges)
- Usage metrics may have delayed reporting (check after ~1 hour)
- Frames overwrite each other (not accumulating storage)
- Monitor daily if using for extended calls

## Optimization Options (if needed)
- Reduce resolution (currently 1280x720)
- Lower frame rate (currently 25 FPS)
- Adjust JPEG quality (currently 70%)
- Implement adaptive quality based on usage