import type { ReactNode } from "react";

interface AuthLayoutProps {
  children: ReactNode;
}

export function AuthLayout({ children }: AuthLayoutProps) {
  return (
    <div className="relative min-h-screen flex items-center justify-center bg-gradient-to-br from-emerald-600 via-teal-600 to-green-500 overflow-hidden py-12 px-4 sm:px-6">
      {/* Animated background blobs */}
      <div
        className="absolute inset-0 overflow-hidden pointer-events-none"
        aria-hidden="true"
      >
        <div className="absolute -top-40 -left-40 w-[28rem] h-[28rem] bg-teal-300 rounded-full mix-blend-multiply blur-3xl opacity-70 animate-blob" />
        <div className="absolute top-0 -right-40 w-[28rem] h-[28rem] bg-emerald-300 rounded-full mix-blend-multiply blur-3xl opacity-70 animate-blob animation-delay-2000" />
        <div className="absolute -bottom-40 left-20 w-[28rem] h-[28rem] bg-green-300 rounded-full mix-blend-multiply blur-3xl opacity-70 animate-blob animation-delay-4000" />
        <div className="absolute bottom-0 right-0 w-[32rem] h-[32rem] bg-emerald-200 rounded-full mix-blend-multiply blur-3xl opacity-60 animate-blob animation-delay-6000" />
      </div>

      {/* Page transition wrapper */}
      <div className="relative w-full max-w-md animate-fade-in-up">
        {children}
      </div>

      {/* Animations */}
      <style>{`
        @keyframes blob {
          0%, 100% { transform: translate(0, 0) scale(1); }
          25%  { transform: translate(30px, -50px) scale(1.1); }
          50%  { transform: translate(-20px, 20px) scale(0.9); }
          75%  { transform: translate(20px, 40px) scale(1.05); }
        }
        @keyframes fadeInUp {
          from {
            opacity: 0;
            transform: translateY(16px);
          }
          to {
            opacity: 1;
            transform: translateY(0);
          }
        }
        .animate-blob {
          animation: blob 20s ease-in-out infinite;
        }
        .animation-delay-2000 {
          animation-delay: 2s;
        }
        .animation-delay-4000 {
          animation-delay: 4s;
        }
        .animation-delay-6000 {
          animation-delay: 6s;
        }
        .animate-fade-in-up {
          animation: fadeInUp 0.4s ease-out;
        }
        @media (prefers-reduced-motion: reduce) {
          .animate-blob {
            animation: none;
          }
          .animate-fade-in-up {
            animation: none;
          }
        }
      `}</style>
    </div>
  );
}
