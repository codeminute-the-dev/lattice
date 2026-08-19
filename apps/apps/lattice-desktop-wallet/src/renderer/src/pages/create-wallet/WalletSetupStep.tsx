import { useState } from 'react';
import { useForm } from '@tanstack/react-form';
import { CheckCircle2, XCircle } from 'lucide-react';
import { displayToFs, isValidFilename } from '../../../../utils/filename-utils';
import WalletSetupHeader from './WalletSetupHeader';
import WalletNameField from './WalletNameField';
import PasswordField from './PasswordField';
import ConfirmPasswordField from './ConfirmPasswordField';
import SecurityNotice from './SecurityNotice';
import ActionButtons from './ActionButtons';
import PasswordStrength from './PasswordStrength';

type WalletSetupStepProps = {
  onCreate: (walletName: string, password: string) => Promise<void>;
  onBack: () => void;
};

export default function WalletSetupStep({ onCreate, onBack }: WalletSetupStepProps) {
  const [showConfirmPassword, setShowConfirmPassword] = useState(false);

  const form = useForm({
    defaultValues: {
      walletName: '',
      password: '',
      confirmPassword: '',
    },
    onSubmit: async ({ value }) => onCreate(displayToFs(value.walletName), value.password),
  });

  function validateWalletName(val: string) {
    if (!val?.trim()) return 'Please enter a wallet name';
    if (!isValidFilename(val)) return 'Invalid wallet name';
  }

  async function validateWalletNameAsync(val: string) {
    const normalized = displayToFs(val);
    const { walletNames } = await window.appBridge.manager.getExistingWallets();
    const equivalentWalletName = walletNames.find(wallet => wallet.toLowerCase() === normalized.toLowerCase());
    if (equivalentWalletName) {
      return `A wallet named "${equivalentWalletName}" exists. Please choose a different name.`;
    }
  }

  const validatePassword = (val: string) => {
    if (!val) return 'Please enter a password';
    if (val.length < 1) return 'Password must be at least 1 character';
  };

  const validateConfirm = (val: string) => {
    const pwd = form.getFieldValue('password');
    if (!val) return 'Please confirm your password';
    if (val !== pwd) return 'Passwords do not match';
  };

  return (
    <div className="mx-auto flex min-h-0 w-full max-w-md flex-col">
      <WalletSetupHeader />
      <form
        onSubmit={e => {
          e.preventDefault();
          form.handleSubmit();
        }}
        className="space-y-4 sm:space-y-6"
      >
        <form.Field
          name="walletName"
          validators={{
            onChange: ({ value }) => validateWalletName(value),
            onChangeAsync: async ({ value }) => validateWalletNameAsync(value),
            onBlurAsync: async ({ value }) => validateWalletNameAsync(value),
            onSubmit: ({ value }) => validateWalletName(value),
            onSubmitAsync: async ({ value }) => validateWalletNameAsync(value),
          }}
        >
          {field => (
            <WalletNameField
              onChange={v => field.handleChange(v)}
              onBlur={field.handleBlur}
              isChecking={Boolean(field.state.meta?.isValidating)}
              error={(field.state.meta?.errors ?? [])[0] ?? null}
            />
          )}
        </form.Field>

        <form.Field
          name="password"
          validators={{ onChange: ({ value }: { value: string }) => validatePassword(value) }}
        >
          {field => (
            <div className="space-y-2">
              <PasswordField onChange={v => field.handleChange(v)} />
              <PasswordStrength password={field.state.value} />
              {(field.state.meta?.errors?.[0] as string | undefined) && (
                <div className="text-xs text-red-400">{field.state.meta.errors[0] as string}</div>
              )}
            </div>
          )}
        </form.Field>

        <form.Field
          name="confirmPassword"
          validators={{
            onChange: ({ value }: { value: string }) => validateConfirm(value),
          }}
        >
          {field => {
            const passwordsMatch = field.state.value === form.getFieldValue('password');
            return (
              <div className="space-y-2">
                <ConfirmPasswordField
                  onChange={v => field.handleChange(v)}
                  showConfirmPassword={showConfirmPassword}
                  setShowConfirmPassword={setShowConfirmPassword}
                  disabled={form.state.isSubmitting}
                />
                {field.state.value && (
                  <div
                    className={`flex items-center gap-1 text-xs ${passwordsMatch ? 'text-green-400' : 'text-red-400'}`}
                  >
                    {passwordsMatch ? (
                      <CheckCircle2 className="h-3 w-3" />
                    ) : (
                      <XCircle className="h-3 w-3" />
                    )}
                    <span>{passwordsMatch ? 'Passwords match' : 'Passwords do not match'}</span>
                  </div>
                )}
              </div>
            );
          }}
        </form.Field>

        <SecurityNotice />

        <form.Subscribe
          selector={s => ({
            isValid: s.isValid,
            isSubmitting: s.isSubmitting,
            isValidating: s.isValidating,
          })}
        >
          {({ isValid, isSubmitting, isValidating }) => (
            <ActionButtons
              onBack={onBack}
              isCreating={isSubmitting}
              isValidating={isValidating}
              isValid={isValid}
            />
          )}
        </form.Subscribe>
      </form>
    </div>
  );
}
