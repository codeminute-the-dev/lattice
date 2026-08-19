interface LogoProps {
    className?: string;
}

export function LogoSmall({ className = 'h-8 w-auto' }: LogoProps = {}) {
    return (
        <svg
            width="376"
            height="310"
            viewBox="0 0 376 310"
            fill="none"
            xmlns="http://www.w3.org/2000/svg"
            className={className}
        >
            <path
                d="M172.982 29.9973H240.204C304 30 347 59 347 108.5C347 154 307 187 240 187V170C284 168 300 142 300 108C300 70 274 47 234 47H218V263H265V280H131V263H172.982V29.9973Z"
                fill="currentColor"
            />
            <path
                d="M135 186.934C69.6493 186.934 29 157.511 29 108.249C29 62.9709 67.7444 29.9969 135 29.9969L135 186.934Z"
                fill="currentColor"
            />
        </svg>
    );
}

export function Logo({ className = 'h-10 w-auto' }: LogoProps = {}) {
    return (
        <svg
            width="1360"
            height="310"
            viewBox="0 0 1360 310"
            fill="none"
            xmlns="http://www.w3.org/2000/svg"
            className={className}
        >
            <path
                d="M1158 47H1121V29.9539H1240V47H1203L1203.19 263H1274L1325 203.947H1331L1318 280H1121V263H1158V47Z"
                fill="currentColor"
            />
            <path
                d="M975 263V280H865V263H892.957V47H857L857 30H964C1024 30 1068 51 1068 98C1068 133 1039 155 1003 164L1108 280H1050L954 168H938V263H975ZM938 47V151.087L958 151C993 151 1021 135 1021 99C1021 63 993 47 958 47H938Z"
                fill="currentColor"
            />
            <path
                d="M677 30H725L821 263H852V280H728.989V263H772L748 205H650L626 263H665V280H586V263H606L690 61L677 30ZM656 188H741L699.212 83.9251L656 188Z"
                fill="currentColor"
            />
            <path
                d="M490 149L514 119H519V196H514L491 166H443.019V263H524L573 209H579L567 280H361.157V263H398V47H361L361.157 29.9544H536L556 19V92H550L515 47H443.019V149H490Z"
                fill="currentColor"
            />
            <path
                d="M172.982 29.9973H240.204C304 30 347 59 347 108.5C347 154 307 187 240 187V170C284 168 300 142 300 108C300 70 274 47 234 47H218V263H265V280H131V263H172.982V29.9973Z"
                fill="currentColor"
            />
            <path
                d="M135 186.934C69.6493 186.934 29 157.511 29 108.249C29 62.9709 67.7444 29.9969 135 29.9969L135 186.934Z"
                fill="currentColor"
            />
        </svg>
    );
}
