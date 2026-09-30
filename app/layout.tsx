import type { Metadata } from 'next';
import { Geist, Geist_Mono } from 'next/font/google';
import './globals.css';

const geistSans = Geist({
  variable: '--font-geist-sans',
  subsets: ['latin'],
});

const geistMono = Geist_Mono({
  variable: '--font-geist-mono',
  subsets: ['latin'],
});

export const metadata: Metadata = {
  metadataBase: new URL('https://sloserve-lab.silky-ibis-9816.chatgpt.site'),
  title: 'SLOServe — Inference Control Plane & Benchmark Lab',
  description:
    'A reproducible lab for measuring and protecting LLM inference latency, goodput, reliability, and cost.',
  openGraph: {
    title: 'SLOServe — Inference Control Plane & Benchmark Lab',
    description:
      'A reproducible Go gateway that protects interactive LLM latency under overload.',
    images: [{ url: '/og.png', width: 1200, height: 630 }],
  },
  twitter: {
    card: 'summary_large_image',
    title: 'SLOServe — Inference Control Plane & Benchmark Lab',
    description:
      'A reproducible Go gateway that protects interactive LLM latency under overload.',
    images: ['/og.png'],
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className="dark">
      <body
        className={`${geistSans.variable} ${geistMono.variable} antialiased`}
      >
        {children}
      </body>
    </html>
  );
}
