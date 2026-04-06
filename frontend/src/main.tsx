import React from 'react'
import {createRoot} from 'react-dom/client'
import './style.css'
import App from './App'

const container = document.getElementById('root')
const root = createRoot(container!)

root.render(
    <React.StrictMode>
        <App/>
    </React.StrictMode>
)

const markAppReady = () => {
    document.body.classList.add('app-ready')
}

const afterNextPaint = (callback: () => void) => {
    requestAnimationFrame(() => {
        requestAnimationFrame(callback)
    })
}

const waitForWindowLoad = () => new Promise<void>((resolve) => {
    if (document.readyState === 'complete') {
        resolve()
        return
    }
    window.addEventListener('load', () => resolve(), { once: true })
})

const waitForFonts = async () => {
    // 폰트 API가 없는 환경 대응
    if (!document.fonts?.ready) {
        return;
    }
    
    try {
        // 최대 1.2초 동안 폰트가 로드되기를 기다림 (모바일 네트워크 고려)
        await Promise.race([
            document.fonts.ready,
            new Promise(resolve => setTimeout(resolve, 1200))
        ]);
    } catch (e) {
        console.warn('Font loading timeout or error', e);
    }
}

const waitForStyles = () => new Promise<void>((resolve) => {
    const check = () => {
        // App.css에 정의된 변수가 로드되었는지 확인
        const bg = getComputedStyle(document.documentElement).getPropertyValue('--accent').trim();
        if (bg) {
            resolve();
        } else {
            requestAnimationFrame(check);
        }
    };
    
    // 최대 1초 대기 후 강제 진행
    const timeout = setTimeout(resolve, 1000);
    check();
})

void Promise.all([
    waitForWindowLoad(), 
    waitForFonts(),
    waitForStyles()
]).finally(() => {
    // 스타일 적용 후 브라우저가 화면을 그릴 시간을 주기 위해 2프레임 정도 기다림
    afterNextPaint(() => markAppReady());
})

if ('serviceWorker' in navigator) {
    window.addEventListener('load', () => {
        void navigator.serviceWorker.register('/sw.js').catch((error) => {
            console.error('Service worker registration failed:', error)
        })
    }, { once: true })
}
