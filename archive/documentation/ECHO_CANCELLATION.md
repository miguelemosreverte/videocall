# Echo Cancellation Options

For future improvement to reduce echo in video calls:

## 1. Browser Built-in (Easiest)
Add to getUserMedia constraints:
```javascript
localStream = await navigator.mediaDevices.getUserMedia({ 
  video: true, 
  audio: {
    echoCancellation: true,
    noiseSuppression: true,
    autoGainControl: true
  }
});
```

## 2. Hardware Solution (Best)
- Use headphones/earbuds
- Use directional microphones
- Increase distance between speakers and mic

## 3. PeerJS Audio Processing
```javascript
peer = new Peer(myUuid, {
  config: {
    iceServers: [...],
    sdpSemantics: 'unified-plan'
  }
});
```

## 4. WebRTC Audio Processing
Configure RTCPeerConnection with audio processing:
```javascript
const constraints = {
  audio: {
    echoCancellation: { exact: true },
    googEchoCancellation: { exact: true },
    googAutoGainControl: { exact: true },
    googNoiseSuppression: { exact: true }
  }
};
```

The current implementation works great - these are just future enhancements!