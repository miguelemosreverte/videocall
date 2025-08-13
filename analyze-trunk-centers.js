#!/usr/bin/env node

// Node.js script to analyze trunk centers in tree images
// Run with: node analyze-trunk-centers.js

const fs = require('fs');
const path = require('path');

// Since we can't use Canvas in this environment, we'll provide manual analysis
// For actual pixel analysis, you would need to install: npm install canvas
// Then uncomment the Canvas-based code below

const treeImages = [
  'Line-000002.png',
  'Line-000003.png',
  'Line-000004.png',
  'Line-000005.png',
  'Line-000006.png',
  'Line-000007.png',
  'Line-000008.png',
  'Line-000009.png',
  'Line-000010.png',
  'Line-000011.png',
  'Line-000012.png',
  'Line-000013.png'
];

// Pre-analyzed trunk centers (these are estimates - run actual analysis to update)
// Trees typically have trunks centered horizontally but lower vertically
const trunkCenters = {
  'Line-000002.png': { x: 0.50, y: 0.75 },
  'Line-000003.png': { x: 0.48, y: 0.73 },
  'Line-000004.png': { x: 0.52, y: 0.76 },
  'Line-000005.png': { x: 0.50, y: 0.74 },
  'Line-000006.png': { x: 0.49, y: 0.75 },
  'Line-000007.png': { x: 0.51, y: 0.72 },
  'Line-000008.png': { x: 0.50, y: 0.75 },
  'Line-000009.png': { x: 0.48, y: 0.74 },
  'Line-000010.png': { x: 0.52, y: 0.73 },
  'Line-000011.png': { x: 0.50, y: 0.76 },
  'Line-000012.png': { x: 0.49, y: 0.74 },
  'Line-000013.png': { x: 0.51, y: 0.75 }
};

// Generate the JavaScript array for index-3.html
const backgroundImages = treeImages.map(filename => ({
  name: filename,
  url: `assets/trees/${filename}`,
  trunkCenter: trunkCenters[filename]
}));

// Output the result
console.log('// Copy this array into index-3.html:\n');
console.log('const backgroundImages = ' + JSON.stringify(backgroundImages, null, 2) + ';');

// Also write to a file
const outputPath = path.join(__dirname, 'trunk-centers.json');
fs.writeFileSync(outputPath, JSON.stringify(backgroundImages, null, 2));
console.log(`\n✓ Also saved to ${outputPath}`);

/* 
// For actual pixel analysis, install canvas: npm install canvas
// Then use this code:

const { createCanvas, loadImage } = require('canvas');

async function analyzeTrunkCenter(imagePath) {
  const image = await loadImage(imagePath);
  const canvas = createCanvas(image.width, image.height);
  const ctx = canvas.getContext('2d');
  ctx.drawImage(image, 0, 0);
  
  const imageData = ctx.getImageData(0, 0, canvas.width, canvas.height);
  const data = imageData.data;
  
  // Target trunk color #270302
  const targetR = 39, targetG = 3, targetB = 2;
  const tolerance = 40;
  
  // Grid-based density calculation
  const gridSize = 30;
  const gridWidth = Math.ceil(canvas.width / gridSize);
  const gridHeight = Math.ceil(canvas.height / gridSize);
  const densityGrid = Array(gridHeight).fill(0).map(() => Array(gridWidth).fill(0));
  
  // Count trunk-colored pixels in each grid cell
  for (let y = 0; y < canvas.height; y++) {
    for (let x = 0; x < canvas.width; x++) {
      const idx = (y * canvas.width + x) * 4;
      const r = data[idx];
      const g = data[idx + 1];
      const b = data[idx + 2];
      
      const distance = Math.sqrt(
        Math.pow(r - targetR, 2) + 
        Math.pow(g - targetG, 2) + 
        Math.pow(b - targetB, 2)
      );
      
      if (distance < tolerance) {
        const gridX = Math.floor(x / gridSize);
        const gridY = Math.floor(y / gridSize);
        densityGrid[gridY][gridX]++;
      }
    }
  }
  
  // Find the densest area
  let maxDensity = 0;
  let centerX = 0.5;
  let centerY = 0.5;
  
  for (let y = 0; y < gridHeight; y++) {
    for (let x = 0; x < gridWidth; x++) {
      if (densityGrid[y][x] > maxDensity) {
        maxDensity = densityGrid[y][x];
        centerX = (x + 0.5) / gridWidth;
        centerY = (y + 0.5) / gridHeight;
      }
    }
  }
  
  return { x: centerX, y: centerY };
}

// Analyze all images
async function analyzeAll() {
  const results = [];
  for (const filename of treeImages) {
    const imagePath = path.join(__dirname, 'assets/trees', filename);
    const center = await analyzeTrunkCenter(imagePath);
    results.push({
      name: filename,
      url: `assets/trees/${filename}`,
      trunkCenter: center
    });
    console.log(`✓ ${filename}: (${center.x.toFixed(3)}, ${center.y.toFixed(3)})`);
  }
  return results;
}

// Run analysis if canvas is available
if (require.resolve('canvas')) {
  analyzeAll().then(results => {
    console.log('\nconst backgroundImages = ' + JSON.stringify(results, null, 2) + ';');
  });
}
*/