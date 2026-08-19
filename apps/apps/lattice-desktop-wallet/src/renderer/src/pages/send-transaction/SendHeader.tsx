import {ArrowLeft} from 'lucide-react';

type SendHeaderProps = {
  title: string;
  onBack: () => void;
};

export default function SendHeader({title, onBack}: SendHeaderProps) {
  return (
    <div className="flex flex-shrink-0 items-center gap-4 border-b border-gray-200 bg-white/80 p-6 shadow-sm backdrop-blur-sm">
      <button onClick={onBack} className="rounded-lg p-2 transition-colors hover:bg-gray-100">
        <ArrowLeft className="h-5 w-5 text-gray-700" />
      </button>
      <h1 className="text-xl font-semibold text-gray-900">{title}</h1>
    </div>
  );
}
