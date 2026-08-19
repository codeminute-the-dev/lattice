type ProgressProps = {
  current: 'wallet-setup' | 'seed-display' | 'seed-verification' | 'complete' | 'intro';
};

export default function Progress({current}: ProgressProps) {
  if (current === 'intro' || current === 'complete') return null;
  
  const steps = ['wallet-setup', 'seed-display', 'seed-verification'];
  const currentIndex = steps.indexOf(current);
  
  return (
    <div className="flex flex-shrink-0 justify-center px-8 pb-4">
      <div className="flex space-x-2">
        {steps.map((stepName, index) => (
          <div
            key={stepName}
            className={`h-2 w-8 rounded-full ${
              stepName === current
                ? 'bg-blue-600'
                : currentIndex > index
                  ? 'bg-green-600'
                  : 'bg-neutral-700'
            }`}
          />
        ))}
      </div>
    </div>
  );
}
