import {useState} from 'react';
import {Info} from 'lucide-react';

type KeyValueRowProps = {
  label: string;
  tooltipContent?: React.ReactNode;
  rightContent: React.ReactNode;
};

export default function KeyValueRow({label, tooltipContent, rightContent}: KeyValueRowProps) {
  const [showTooltip, setShowTooltip] = useState(false);
  const hasTooltip = tooltipContent !== undefined;
  return (
    <div className="flex items-center justify-between">
      <div className="relative flex items-center gap-1">
        <span className="text-sm text-gray-600">{label}</span>
        {hasTooltip && (
          <button
            onMouseEnter={() => setShowTooltip(true)}
            onMouseLeave={() => setShowTooltip(false)}
            className="text-gray-500 transition-colors hover:text-gray-700"
          >
            <Info className="h-3 w-3" />
          </button>
        )}
        {showTooltip && (
          <div className="absolute left-0 top-6 z-10 w-64 rounded-lg border border-gray-300 bg-white p-2 text-xs text-gray-700 shadow-lg">
            {tooltipContent}
          </div>
        )}
      </div>
      <div className="text-right">
        <span className="font-semibold text-gray-900">{rightContent}</span>
      </div>
    </div>
  );
}
