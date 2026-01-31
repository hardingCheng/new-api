import React, { useEffect, useRef } from 'react';
import { useActualTheme } from '../../context/Theme';

const TechBackground = () => {
  const canvasRef = useRef(null);
  const theme = useActualTheme();
  const isDark = theme === 'dark';

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const ctx = canvas.getContext('2d');
    let animationFrameId;
    let particles = [];
    let matrixColumns = [];
    let dataStreams = [];
    let pulseRings = [];

    // 粒子类 - 用于神经网络效果
    class Particle {
      constructor() {
        this.reset();
      }

      reset() {
        this.x = Math.random() * canvas.width;
        this.y = Math.random() * canvas.height;
        this.vx = (Math.random() - 0.5) * 0.8;
        this.vy = (Math.random() - 0.5) * 0.8;
        this.radius = Math.random() * 2 + 1;
        this.opacity = Math.random() * 0.5 + 0.3;
      }

      update() {
        this.x += this.vx;
        this.y += this.vy;

        if (this.x < 0 || this.x > canvas.width) this.vx *= -1;
        if (this.y < 0 || this.y > canvas.height) this.vy *= -1;
      }

      draw() {
        ctx.beginPath();
        ctx.arc(this.x, this.y, this.radius, 0, Math.PI * 2);
        const gradient = ctx.createRadialGradient(
          this.x,
          this.y,
          0,
          this.x,
          this.y,
          this.radius * 3
        );
        if (isDark) {
          gradient.addColorStop(0, `rgba(96, 165, 250, ${this.opacity})`);
          gradient.addColorStop(1, 'rgba(96, 165, 250, 0)');
        } else {
          gradient.addColorStop(0, `rgba(59, 130, 246, ${this.opacity * 0.6})`);
          gradient.addColorStop(1, 'rgba(59, 130, 246, 0)');
        }
        ctx.fillStyle = gradient;
        ctx.fill();
      }
    }

    // 数字雨效果
    const initMatrix = () => {
      const fontSize = 14;
      const columnCount = Math.floor(canvas.width / fontSize);
      matrixColumns = [];
      for (let i = 0; i < columnCount; i++) {
        matrixColumns.push({
          x: i * fontSize,
          y: Math.random() * canvas.height,
          speed: Math.random() * 2 + 1,
          chars: [],
        });
      }
    };

    const drawMatrix = () => {
      const chars = '01アイウエオカキクケコサシスセソABCDEFGHIJKLMNOPQRSTUVWXYZ';
      ctx.font = '14px monospace';

      matrixColumns.forEach((column) => {
        if (Math.random() > 0.98) {
          const char = chars[Math.floor(Math.random() * chars.length)];
          ctx.fillStyle = isDark
            ? `rgba(34, 197, 94, ${Math.random() * 0.5 + 0.3})`
            : `rgba(34, 197, 94, ${Math.random() * 0.3 + 0.2})`;
          ctx.fillText(char, column.x, column.y);

          column.y += column.speed;
          if (column.y > canvas.height) {
            column.y = 0;
          }
        }
      });
    };

    // 绘制数据流动效果
    const initDataStreams = () => {
      dataStreams = [];
      for (let i = 0; i < 5; i++) {
        dataStreams.push({
          x: Math.random() * canvas.width,
          y: -50,
          speed: Math.random() * 3 + 2,
          length: Math.random() * 100 + 50,
          opacity: Math.random() * 0.5 + 0.3,
        });
      }
    };

    const drawDataStreams = () => {
      dataStreams.forEach((stream) => {
        const gradient = ctx.createLinearGradient(
          stream.x,
          stream.y,
          stream.x,
          stream.y + stream.length
        );
        if (isDark) {
          gradient.addColorStop(0, `rgba(168, 85, 247, ${stream.opacity})`);
          gradient.addColorStop(0.5, `rgba(96, 165, 250, ${stream.opacity * 0.6})`);
          gradient.addColorStop(1, 'rgba(96, 165, 250, 0)');
        } else {
          gradient.addColorStop(0, `rgba(147, 51, 234, ${stream.opacity * 0.6})`);
          gradient.addColorStop(0.5, `rgba(59, 130, 246, ${stream.opacity * 0.4})`);
          gradient.addColorStop(1, 'rgba(59, 130, 246, 0)');
        }

        ctx.strokeStyle = gradient;
        ctx.lineWidth = 2;
        ctx.beginPath();
        ctx.moveTo(stream.x, stream.y);
        ctx.lineTo(stream.x, stream.y + stream.length);
        ctx.stroke();

        stream.y += stream.speed;
        if (stream.y > canvas.height + stream.length) {
          stream.y = -stream.length;
          stream.x = Math.random() * canvas.width;
        }
      });
    };

    // 绘制脉冲圆环
    const initPulseRings = () => {
      pulseRings = [];
      for (let i = 0; i < 3; i++) {
        pulseRings.push({
          x: Math.random() * canvas.width,
          y: Math.random() * canvas.height,
          radius: 0,
          maxRadius: Math.random() * 200 + 100,
          speed: Math.random() * 2 + 1,
          opacity: 1,
        });
      }
    };

    const drawPulseRings = () => {
      pulseRings.forEach((ring) => {
        const opacity = (1 - ring.radius / ring.maxRadius) * 0.5;
        ctx.beginPath();
        ctx.arc(ring.x, ring.y, ring.radius, 0, Math.PI * 2);
        ctx.strokeStyle = isDark
          ? `rgba(96, 165, 250, ${opacity})`
          : `rgba(59, 130, 246, ${opacity * 0.6})`;
        ctx.lineWidth = 2;
        ctx.stroke();

        ring.radius += ring.speed;
        if (ring.radius > ring.maxRadius) {
          ring.radius = 0;
          ring.x = Math.random() * canvas.width;
          ring.y = Math.random() * canvas.height;
        }
      });
    };

    // 绘制神经网络连接
    const drawConnections = () => {
      const maxDistance = 150;

      for (let i = 0; i < particles.length; i++) {
        for (let j = i + 1; j < particles.length; j++) {
          const dx = particles[i].x - particles[j].x;
          const dy = particles[i].y - particles[j].y;
          const distance = Math.sqrt(dx * dx + dy * dy);

          if (distance < maxDistance) {
            const opacity = ((1 - distance / maxDistance) * 0.4).toFixed(2);
            ctx.beginPath();
            ctx.moveTo(particles[i].x, particles[i].y);
            ctx.lineTo(particles[j].x, particles[j].y);
            ctx.strokeStyle = isDark
              ? `rgba(96, 165, 250, ${opacity})`
              : `rgba(59, 130, 246, ${opacity * 0.6})`;
            ctx.lineWidth = 0.5;
            ctx.stroke();
          }
        }
      }
    };

    const resizeCanvas = () => {
      canvas.width = window.innerWidth;
      canvas.height = document.documentElement.scrollHeight;
      initMatrix();
    };

    // 初始化
    resizeCanvas();
    window.addEventListener('resize', resizeCanvas);

    // 初始化粒子
    const particleCount = Math.min(Math.floor((canvas.width * canvas.height) / 20000), 80);
    for (let i = 0; i < particleCount; i++) {
      particles.push(new Particle());
    }

    initDataStreams();
    initPulseRings();

    const animate = () => {
      // 半透明清除，产生拖尾效果
      ctx.fillStyle = isDark ? 'rgba(15, 23, 42, 0.1)' : 'rgba(248, 250, 252, 0.1)';
      ctx.fillRect(0, 0, canvas.width, canvas.height);

      drawMatrix();
      drawPulseRings();
      drawDataStreams();

      particles.forEach((particle) => {
        particle.update();
        particle.draw();
      });

      drawConnections();

      animationFrameId = requestAnimationFrame(animate);
    };

    animate();

    return () => {
      window.removeEventListener('resize', resizeCanvas);
      cancelAnimationFrame(animationFrameId);
    };
  }, [isDark]);

  return (
    <div className='fixed top-0 left-0 w-full h-full pointer-events-none z-0 overflow-hidden'>
      {/* 渐变背景 */}
      <div
        className='absolute inset-0'
        style={{
          background: isDark
            ? 'linear-gradient(180deg, #0f172a 0%, #1e293b 50%, #0f172a 100%)'
            : 'linear-gradient(180deg, #f8fafc 0%, #e0f2fe 50%, #f8fafc 100%)',
        }}
      />
      {/* Canvas 动画层 */}
      <canvas ref={canvasRef} className='absolute inset-0 w-full h-full' />
    </div>
  );
};

export default TechBackground;
